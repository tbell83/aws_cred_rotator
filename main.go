package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
)

var print = fmt.Println

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func awsSession(profile string) *session.Session {
	if profile == "" {
		profile = "default"
	}
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Profile: profile,
	}))
	return sess
}

func readCreds(credsFilePath string) map[string]map[string]string {
	credsFile, err := os.Open(credsFilePath)
	check(err)
	blocks := make(map[string]map[string]string)
	regex, err := regexp.Compile("^\\[.*\\]")
	check(err)
	scanner := bufio.NewScanner(credsFile)
	var profileName string
	for scanner.Scan() {
		if scanner.Text() != "" {
			if regex.MatchString(scanner.Text()) {
				profileName = strings.Replace(strings.Replace(scanner.Text(), "[", "", -1), "]", "", -1)
				blocks[profileName] = make(map[string]string, 2)
			} else {
				split := strings.Split(scanner.Text(), " = ")
				blocks[profileName][split[0]] = split[1]
			}
		} else {
			profileName = ""
		}
	}
	return blocks
}

func writeCreds(credsFilePath string, creds map[string]map[string]string) {
	file, err := os.Create(credsFilePath + ".tmp")
	check(err)
	defer file.Close()

	writer := bufio.NewWriter(file)
	for profile, credmap := range creds {
		writer.WriteString("[" + profile + "]\n")
		for key, value := range credmap {
			writer.WriteString(key + " = " + value + "\n")
		}
		writer.WriteString("\n")
	}
	writer.Flush()

	// move old credsFile
	err = os.Rename(credsFilePath, credsFilePath+".bak")
	check(err)

	// rename new credsFile
	err = os.Rename(credsFilePath+".tmp", credsFilePath)
	check(err)
}

func getNewCreds(sess *session.Session) *iam.AccessKey {
	iamClient := iam.New(sess)
	currentKeys, err := iamClient.ListAccessKeys(&iam.ListAccessKeysInput{})
	check(err)

	// make sure there is only one set of creds
	var currentKeyID *string
	if len(currentKeys.AccessKeyMetadata) > 1 {
		print("There is more than 1 key defined for this profile")
		os.Exit(1)
	} else {
		currentKeyID = currentKeys.AccessKeyMetadata[0].AccessKeyId
	}

	// Create new creds
	newCreds, err := iamClient.CreateAccessKey(&iam.CreateAccessKeyInput{})
	check(err)

	// remove old creds
	result, err := iamClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
		AccessKeyId: currentKeyID,
	})
	check(err)
	print(result)

	return newCreds.AccessKey
}

func main() {
	profileFlag := flag.String("profile", "default", "AWS profile, defaults to 'default'")
	user, err := user.Current()
	check(err)
	credsFilePath := user.HomeDir + "/.aws/credentials"
	flag.Parse()
	profile := *profileFlag

	sess := awsSession(profile)
	creds := readCreds(credsFilePath)
	newCreds := getNewCreds(sess)
	creds[profile]["aws_access_key_id"] = *newCreds.AccessKeyId
	creds[profile]["aws_secret_access_key"] = *newCreds.SecretAccessKey
	writeCreds(credsFilePath, creds)
}
