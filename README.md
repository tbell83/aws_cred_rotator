# aws_cred_rotator

rotate creds in a strongly typed language

## Usage

```text
Usage of ./aws_cred_rotator:
  -config-dir string
        Path for AWS CLI credentials file. (default "~/.aws/")
  -profile string
        AWS profile for which to rotate credentials. Use comma-delimited string to rotate multiple profiles. (default "default")
  -account-ids string
        AWS Account IDs for which to allow rotation of credentials. Use comma-delimited string to rotate credentials for multiple AWS accounts. (default "false" allows rotation on any AWS account ID.)
```