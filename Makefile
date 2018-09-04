PROJECT = eksClient
FUNCTION = $(PROJECT)
REGION = us-east-1

.phony: clean

clean:
	rm -f main main.zip -

build:
	GOOS=linux GOARCH=amd64 go build -o main main.go
	zip main.zip main

create:
	aws lambda create-function \
		--handler main \
	        --function-name $(FUNCTION) \
		--region $(REGION) \
		--role arn:aws:iam::991225764181:role/KubernetesAdmin \
		--zip-file fileb://main.zip \
		--runtime go1.x \
		--timeout 30 \
		--memory-size 128

config:
	aws lambda update-function-configuration \
		--handler main \
	        --function-name $(FUNCTION) \
		--region $(REGION) \
		--role arn:aws:iam::991225764181:role/LambdaEKSClientRole \
		--runtime go1.x \
		--timeout 30 \
		--memory-size 256 \
		--environment Variables="{cluster=devel,region=us-west-2,arn=arn:aws:iam::991225764181:role/KubernetesAdmin}"

update:
	aws lambda update-function-code \
		--function-name $(FUNCTION) \
		--zip-file fileb://main.zip \
		--publish

delete:
	aws lambda delete-function --function-name $(FUNCTION)

invoke:
	aws lambda invoke --function-name $(FUNCTION) \
		--region $(REGION) \
		--log-type Tail - \
		| jq '.LogResult' -r | base64 -D
		
