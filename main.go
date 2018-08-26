/*
Nick Brandaleone
August 2018

This code demonstates how to interact with an Elastic Kubernetes Service, using an AWS Lambda function.
This supports the following blog entry: http://www.nickaws.net/aws/2018/08/26/Interacting-with-EKS-via-Lambda.html

Parts of the code leverages the Kubernetes Go client, and the AWS Go SDK/AWS Go Lambda SDK.
*/

/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"io/ioutil"
	"log"
	"net/http"
	"text/template"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/aws/endpoints"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/eks"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func check(e error) {
	if e != nil {
		log.Fatalln(e)
	}
}

func getClusterInfo() (string, string, string) {
	// Using the SDK's default configuration, loading additional config
	// and credentials values from the environment variables, shared
	// credentials, and shared configuration files
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	// Get the region and cluster name from env variables. Hard-coded for now.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/endpoints/#pkg-constants
	cfg.Region = endpoints.UsWest2RegionID
	svc := eks.New(cfg)
	input := &eks.DescribeClusterInput{
		Name: aws.String("eks"),
	}

	// Prepare request to EKS endpoint
	// Code from: https://github.com/aws/aws-sdk-go/blob/master/service/eks/examples_test.go
	req := svc.DescribeClusterRequest(input)
	result, err := req.Send()
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case eks.ErrCodeResourceNotFoundException:
				fmt.Println(eks.ErrCodeResourceNotFoundException, aerr.Error())
			case eks.ErrCodeClientException:
				fmt.Println(eks.ErrCodeClientException, aerr.Error())
			case eks.ErrCodeServerException:
				fmt.Println(eks.ErrCodeServerException, aerr.Error())
			case eks.ErrCodeServiceUnavailableException:
				fmt.Println(eks.ErrCodeServiceUnavailableException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			} // switch
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		//return
		panic("a problem")
	}

	return *result.Cluster.Name, *result.Cluster.Endpoint, *result.Cluster.CertificateAuthority.Data
}

func getAuthenticator() {
	// This function gets the "aws-iam-authenticator" binary, and installs it in /tmp

	const authURL = "https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-07-26/bin/linux/amd64/aws-iam-authenticator"
	var netClient = &http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := netClient.Get(authURL)
	check(err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	check(err)

	// write binary to /tmp
	err = ioutil.WriteFile("/tmp/aws-iam-authenticator", body, 0755)
	check(err)
}

func buildConfig() {
	// This function creates a KUBECONFIG file, using a template structure

	t := template.New("KUBECONFIG")
	text := `
apiVersion: v1
clusters:
- cluster:
    server: {{.Server}}
    certificate-authority-data: {{.CertificateAuthority}}
  name: kubernetes
contexts:
- context:
    cluster: kubernetes
    user: aws
  name: aws
current-context: aws
kind: Config
preferences: {}
users:
- name: aws
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1alpha1
      command: /tmp/aws-iam-authenticator
      args:
        - "token"
        - "-i"
        - "{{.Name}}"
        - "-r"
        - "{{.Role}}"
`

	t, err := t.Parse(text)
	check(err)

	type EKSConfig struct {
		Name                 string
		Role                 string
		Server               string
		CertificateAuthority string
	}

	// Get EKS cluster details. TODO: Error checking
	n, e, ca := getClusterInfo()
	config := EKSConfig{n, "arn:aws:iam::991225764181:role/KubernetesAdmin", e, ca}

	// Open a new file for reading/writing only
	file, err := os.OpenFile(
		"/tmp/KUBECONFIG",
		os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
		0666,
	)
	check(err)
	defer file.Close()

	// Write kubectl configuration file to /tmp
	//err = t.Execute(os.Stdout, config)
	err = t.Execute(file, config)
	check(err)
}

// https://github.com/kubernetes/client-go/blob/master/examples/out-of-cluster-client-configuration/main.go
func LambdaHandler() {
	getAuthenticator()
	buildConfig()

	var kubeconfig *string
	kubeconfig = flag.String("kubeconfig", filepath.Join("/tmp", "KUBECONFIG"), "(optional) absolute path to the kubeconfig file")
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	// Examples for error handling:
	// - Use helper functions like e.g. errors.IsNotFound()
	// - And/or cast to StatusError and use its properties like e.g. ErrStatus.Message
	namespace := "default"
	pod := "example-xxxxx"
	_, err = clientset.CoreV1().Pods(namespace).Get(pod, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		fmt.Printf("Pod %s in namespace %s not found\n", pod, namespace)
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting pod %s in namespace %s: %v\n",
			pod, namespace, statusError.ErrStatus.Message)
	} else if err != nil {
		panic(err.Error())
	} else {
		fmt.Printf("Found pod %s in namespace %s\n", pod, namespace)
	}
}

func main() {
	lambda.Start(LambdaHandler)
}
