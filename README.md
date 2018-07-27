# aws_cred_rotator

rotate creds in a strongly typed language

## Usage

```text
Usage of ./aws_cred_rotator:
  -account-ids string
        AWS Account IDs for which to allow rotation of credentials. Use comma-delimited string to rotate credentials for multiple AWS accounts.
  -config-dir string
        Path for AWS CLI config files. (default "~/.aws/")
  -debug
        Turn on debug output.
  -keyAge float
        Only rotate creds if they exceed this age, in days.
  -profile string
        AWS profile for which to rotate credentials. Use comma-delimited string to rotate multiple profiles. To rotate all profiles pass 'all'. (default "default")
```