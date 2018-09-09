# citium
[![Go Report Card](https://goreportcard.com/badge/github.com/meomap/citium)](https://goreportcard.com/report/github.com/meomap/citium) [![Build Status](https://travis-ci.org/meomap/citium.svg?branch=master)](https://travis-ci.org/meomap/zeno) [![Coverage Status](https://coveralls.io/repos/github/meomap/citium/badge.svg?branch=master)](https://coveralls.io/github/meomap/citium?branch=master)

Serverless function to trigger a scheduled HTTP request

## Requirements

* AWS CLI already configured with AdministratorAccess permission [IAM Admin](https://docs.aws.amazon.com/IAM/latest/UserGuide/getting-started_create-admin-group.html)
* AWS SAM CLI installed [SAM CLI](https://github.com/awslabs/aws-sam-cli)

## Setup process

### Building

Compile binary function:

```shell
make build
```

### Packaging & Deployment

Prepare a `S3 bucket` to upload the binary Lambda function:

```bash
aws s3 mb s3://citium-builds --profile adminuser
```

Package Lambda function to S3:

```bash
sam package \
    --template-file template.yaml \
    --s3-bucket citium-builds \
    --output-template-file template.output.yaml \
    --profile adminuser
```

The returned file `template.output.yaml` now should contain the `CodeUri` that points to the artifact to be deployed.

Create a Cloudformation Stack and deploy SAM resources:

```bash
sam deploy \
    --template-file template.output.yaml \
    --stack-name citium-serverless \
    --capabilities CAPABILITY_IAM \
    --profile adminuser
```

After the deployment is complete, run the following command to retrieve stack info:

```bash
aws cloudformation describe-stacks --stack-name citium-serverless
``` 

The dynamodb table name for storing requests is `citium_schedule` which could be overridden at packing step.

```yaml
...
Parameters:
  ScheduleTableName:
    Type: String
    Description: Name of the dynamodb table to be created & used by function
    Default: citium_schedule
```

Default checking interval is 5 minutes.

```yaml
...
      Events:
        PeriodicCheck:
          Type: Schedule
          Properties:
            Schedule: rate(5 minutes)
```


### Local development

**Invoking function locally**

```bash
sam local invoke TriggerAPIFunction  --no-event --env-vars env.json --debug
```

**NOTE:** Template file should be modified before hand with added property `CodeUri` pointing to current dir

```yaml
...
  TriggerAPIFunction:
    Type: AWS::Serverless::Function
    Properties:
      CodeUri: ./
      Handler: citium 
```

## Usage

### Build CLI tool

Compile cli tool to schedule requests:

```shell
make build-tools
```

### Schedule New Request

If `persistent=false` then the scheduled request will be removed after successfully executed:

```bash
./citium-cli \
    -action=create \
    -table=citium_schedule \
    -id=test-post-request \
    -freeze=30m \
    -method=POST \
    -url=http://example.com \
    -headers=Content-Type:application/json \
    -persistent=true
```

### Lock Request

To safely halt request execution, for the case of execution failure that needs manual intervention:

```bash
./citium-cli \
    -action=unlock \
    -table=citium_schedule \
    -id=test-delete-resource
```

### Unlock Request

To release the execution lock:

```bash
./citium-cli \
    -action=lock \
    -table=citium_schedule \
    -id=test-delete-resource
```

## Extra options

Optional extra values for API request authorization, base url, etc are configured via environment variables:

```yam
...
    Environment:
      Variables:
        TABLE_NAME: !Ref ScheduleTableName
        BASE_URL: ""
        API_TOKEN: ""
        USER_AGENT: citium/0.0.1
```
