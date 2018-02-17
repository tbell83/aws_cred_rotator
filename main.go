package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
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
		SharedConfigState: session.SharedConfigEnable,
		Profile:           profile,
	}))
	return sess
}

func findCreds(configDir string) []string {
	var configFiles = [2]string{"config", "credentials"}
	files := make([]string, 0)
	for i := 0; i < 2; i++ {
		_, err := os.Open(configDir + configFiles[i])
		if err == nil {
			files = append(files, configDir+configFiles[i])
		}
	}
	return files
}

func readCreds(configFiles []string) map[string]map[string]string {
	blocks := make(map[string]map[string]string)
	regex, err := regexp.Compile("^\\[.*\\]")
	check(err)

	for i := 0; i < len(configFiles); i++ {
		configFile := configFiles[i]
		file, err := os.Open(configFile)
		check(err)

		scanner := bufio.NewScanner(file)
		var profileName string
		for scanner.Scan() {
			if scanner.Text() != "" {
				if regex.MatchString(scanner.Text()) {
					profileName = strings.Replace(strings.Replace(strings.Replace(scanner.Text(), "[", "", -1), "]", "", -1), "profile ", "", 1)
					if _, ok := blocks[profileName]; !ok {
						blocks[profileName] = make(map[string]string, 2)
					}
				} else {
					split := strings.Split(scanner.Text(), "=")
					if len(split) != 2 {
						split = append(split, "")
						split[0] = strings.Replace(split[0], "=", "", 1)
					}
					for i2 := 0; i2 < len(split); i2++ {
						split[i2] = strings.Replace(split[i2], " ", "", -1)
					}
					blocks[profileName][split[0]] = split[1]
				}
			} else {
				profileName = ""
			}
		}
	}
	return blocks
}

func backup(configFiles []string) {
	for i := 0; i < len(configFiles); i++ {
		configFile := configFiles[i]
		src, err := os.Open(configFile)
		check(err)
		defer src.Close()

		dst, err := os.Create(configFile + ".bak")
		check(err)
		defer dst.Close()

		_, err = io.Copy(dst, src)
		check(err)

		err = dst.Sync()
		check(err)
	}
}

func writeCreds(configPath string, creds map[string]map[string]string) {
	files := [2]string{"config", "credentials"}
	for i := 0; i < len(files); i++ {
		filename := files[i]
		file, err := os.Create(configPath + filename + ".tmp")
		check(err)
		defer file.Close()

		writer := bufio.NewWriter(file)
		for profile, credmap := range creds {
			if filename == "config" && profile != "default" {
				writer.WriteString("[profile " + profile + "]\n")
			} else {
				writer.WriteString("[" + profile + "]\n")
			}
			for key, value := range credmap {
				if filename == "config" && key != "aws_secret_access_key" && key != "aws_access_key_id" {
					writer.WriteString(key + "=" + value + "\n")
				} else if filename == "credentials" && (key == "aws_secret_access_key" || key == "aws_access_key_id") {
					writer.WriteString(key + "=" + value + "\n")
				}
			}
			writer.WriteString("\n")
		}
		writer.Flush()

		// rename new credsFile
		err = os.Rename(configPath+filename+".tmp", configPath+filename)
		check(err)
	}
}

func getNewCreds(sess *session.Session) *iam.AccessKey {
	iamClient := iam.New(sess)
	currentKeys, err := iamClient.ListAccessKeys(&iam.ListAccessKeysInput{})
	check(err)

	// make sure there is only one set of creds
	var currentKeyID *string
	if len(currentKeys.AccessKeyMetadata) > 1 {
		print("There is more than 1 key defined for this profile, nothing has been changed, exiting.")
		os.Exit(1)
	} else {
		currentKeyID = currentKeys.AccessKeyMetadata[0].AccessKeyId
	}

	// Create new creds
	newCreds, err := iamClient.CreateAccessKey(&iam.CreateAccessKeyInput{})
	check(err)

	// remove old creds
	_, err = iamClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
		AccessKeyId: currentKeyID,
	})
	check(err)

	return newCreds.AccessKey
}

func main() {
	profileFlag := flag.String("profile", "default", "AWS profile for which to rotate credentials. Use comma-delimited string to rotate multiple profiles.")
	awsCredsFileFlag := flag.String("creds-file", "~/.aws/", "Path for AWS CLI config files.")
	flag.Parse()
	profiles := strings.Split(*profileFlag, ",")
	awsCredsFile := *awsCredsFileFlag

	user, err := user.Current()
	check(err)

	credsFilePath := strings.Replace(awsCredsFile, "~", user.HomeDir, 1)
	credsFiles := findCreds(credsFilePath)

	backup(credsFiles)

	creds := readCreds(credsFiles)
	for i := 0; i < len(profiles); i++ {
		profile := profiles[i]
		sess := awsSession(profile)
		newCreds := getNewCreds(sess)
		creds[profile]["aws_access_key_id"] = *newCreds.AccessKeyId
		creds[profile]["aws_secret_access_key"] = *newCreds.SecretAccessKey
		print("Successfully rolled creds for " + profile)
	}
	writeCreds(credsFilePath, creds)
}
