/*
Nick Brandaleone
August 2018

This code demonstates how to interact with an Elastic Kubernetes Service, using an AWS Lambda function.
This supports the following blog entry: http://www.nickaws.net/aws/2018/08/26/Interacting-with-EKS-via-Lambda.html

Parts of the code leverages the Kubernetes Go client, and the AWS Go SDK/AWS Go Lambda SDK.
https://github.com/kubernetes/client-go/blob/master/examples/out-of-cluster-client-configuration/main.go
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
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"

	"github.com/kubernetes-sigs/aws-iam-authenticator/pkg/token"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Cluster struct {
	Name     string
	Endpoint string
	CA       []byte
	Role     string
}

func check(e error) {
	if e != nil {
		log.Fatalln(e)
	}
}

func getClusterDetails(name string, role string, region string) (*Cluster, error) {
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	svc := eks.New(sess)
	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}

	result, err := svc.DescribeCluster(input)
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
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return nil, err
	}
	// fmt.Println(result)

	// The CA data comes base64 encoded string inside a JSON object { "Data": "..." }
	ca, err := base64.StdEncoding.DecodeString(*result.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, err
	}

	return &Cluster{
		Name:     *result.Cluster.Name,
		Endpoint: *result.Cluster.Endpoint,
		CA:       ca,
		Role:     role,
	}, nil
}

func (c *Cluster) AuthToken() (string, error) {

	// Init a new aws-iam-authenticator token generator
	gen, err := token.NewGenerator(false)
	if err != nil {
		return "", err
	}

	// Use the current IAM credentials to obtain a K8s bearer token
	tok, err := gen.GetWithRole(c.Name, c.Role)
	if err != nil {
		return "", err
	}

	return tok, nil
}

func LambdaHandler() {
	// Capture ENV variables
	name := os.Getenv("cluster")
	if len(name) < 1 {
		panic("Unable to grab cluster name from ENVIRONMENT variable")
	}
	role := os.Getenv("arn")
	if len(role) < 1 {
		panic("Unable to grab role ARN from ENVIRONMENT variable")
	}
	// Get Region info from ENV or lambda env
	var region string
	region = os.Getenv("region")
	if len(region) < 1 {
		region = os.Getenv("AWS_REGION")
	}

	// Get EKS cluster details
	cluster, err := getClusterDetails(name, role, region)
	fmt.Printf("Amazon EKS Cluster: %s (%s)\n", cluster.Name, cluster.Endpoint)

	// Use the aws-iam-authenticator to fetch a K8s authentication bearer token
	token, err := cluster.AuthToken()
	if err != nil {
		panic("Failed to obtain token from aws-iam-authenticator, " + err.Error())
	}
	// fmt.Printf("Bearer Token: %s\n", token)

	// Create a new K8s client set using the Amazon EKS cluster details
	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host:        cluster.Endpoint,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: cluster.CA,
		},
	})
	if err != nil {
		panic("Failed to create new k8s client, " + err.Error())
	}

	// Now, call kubernetes cluster API server
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
