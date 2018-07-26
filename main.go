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
	"github.com/aws/aws-sdk-go/service/sts"
)

var print = fmt.Println
var loggingEnabled bool

func log(output interface{}) {
	if loggingEnabled {
		print(output)
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func awsSession(profile string) *session.Session {
	// get aws sdk session for given profile
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Profile:           profile,
	}))
	return sess
}

func validateSession(sess *session.Session, accountIds []string) bool {
	// get sts client
	stsClient := sts.New(sess)
	input := &sts.GetCallerIdentityInput{}
	result, err := stsClient.GetCallerIdentity(input)
	if err != nil {
		return false
	}

	if len(accountIds) == 0 {
		return true
	}

	for _, accountID := range accountIds {
		if accountID == *result.Account {
			return true
		}
	}
	return false
}

func findCreds(configDir string) []string {
	// find aws config files in given path
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

func readCreds(configFiles []string) map[string]map[string]interface{} {
	// instantiate aws config object
	blocks := make(map[string]map[string]interface{})

	// set up regex objects
	blockNameRegex, err := regexp.Compile("^\\[.*\\]")
	check(err)
	subBlockRegex, err := regexp.Compile("^[A-Z,a-z,0-9]*=$")
	check(err)
	subValueRegex, err := regexp.Compile("^\\s.*=.*$")
	check(err)

	// iterate through config files
	for i := 0; i < len(configFiles); i++ {
		configFile := configFiles[i]
		file, err := os.Open(configFile)
		check(err)

		scanner := bufio.NewScanner(file)
		var profileName string
		var subBlock map[string]string
		var subBlockName string
		subBlockActive := false
		for scanner.Scan() {
			if scanner.Text() != "" {
				if blockNameRegex.MatchString(scanner.Text()) {
					profileName = strings.Replace(strings.Replace(strings.Replace(scanner.Text(), "[", "", -1), "]", "", -1), "profile ", "", 1)
					if _, ok := blocks[profileName]; !ok {
						blocks[profileName] = make(map[string]interface{}, 2)
					}
				} else if subBlockRegex.MatchString(scanner.Text()) {
					subBlockActive = true
					split := strings.Split(scanner.Text(), "=")
					subBlockName = split[0]
					blocks[profileName][subBlockName] = make(map[string]string, 2)
				} else if subValueRegex.MatchString(scanner.Text()) && subBlockActive {
					split := strings.Split(scanner.Text(), "=")
					subBlock = blocks[profileName][subBlockName].(map[string]string)
					subKey := strings.Replace(split[0], "\t", "", -1)
					subBlock[subKey] = split[1]
					blocks[profileName][subBlockName] = subBlock
				} else {
					subBlockActive = false
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
	// make backups of all available config files
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

func writeCreds(configPath string, creds map[string]map[string]interface{}) {
	files := [2]string{"config", "credentials"}
	for i := 0; i < len(files); i++ {
		filename := files[i]

		// create tmp config file
		file, err := os.Create(configPath + filename + ".tmp")
		check(err)
		defer file.Close()

		writer := bufio.NewWriter(file)

		// iterate through profiles in aws config
		for profile, credmap := range creds {
			if filename == "config" && profile != "default" {
				writer.WriteString("[profile " + profile + "]\n")
			} else {
				writer.WriteString("[" + profile + "]\n")
			}
			for key, value := range credmap {
				// only write credentials to credentials file, everthing else to config
				if filename == "config" && key != "aws_secret_access_key" && key != "aws_access_key_id" {
					if str, ok := value.(string); ok {
						writer.WriteString(key + "=" + str + "\n")
					} else {
						writer.WriteString(key + "=" + "\n")
						for subKey, subValue := range value.(map[string]string) {
							writer.WriteString("\t" + subKey + "=" + subValue + "\n")
						}
					}
				} else if filename == "credentials" && (key == "aws_secret_access_key" || key == "aws_access_key_id") {
					if str, ok := value.(string); ok {
						writer.WriteString(key + "=" + str + "\n")
					} else {
						writer.WriteString(key + "=" + "\n")
						for subKey, subValue := range value.(map[string]string) {
							writer.WriteString("\t" + subKey + "=" + subValue + "\n")
						}
					}
				}
			}
			writer.WriteString("\n")
		}
		writer.Flush()

		// mv tmp files to config files
		err = os.Rename(configPath+filename+".tmp", configPath+filename)
		check(err)
	}
}

func checkCreds(sess *session.Session) bool {
	// get iam client
	iamClient := iam.New(sess)

	// get access keys
	currentKeys, err := iamClient.ListAccessKeys(&iam.ListAccessKeysInput{})
	check(err)

	// make sure there is only one set of creds
	if len(currentKeys.AccessKeyMetadata) > 1 {
		print("There is more than 1 key defined for this profile, nothing has been changed, exiting.")
		os.Exit(1)
	}

	return true
}

func getNewCreds(sess *session.Session) *iam.AccessKey {
	// get iam client
	iamClient := iam.New(sess)

	// get access keys
	currentKeys, err := iamClient.ListAccessKeys(&iam.ListAccessKeysInput{})
	check(err)

	currentKeyID := currentKeys.AccessKeyMetadata[0].AccessKeyId

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

func getProfileNames(creds map[string]map[string]interface{}) []string {
	profileNames := make([]string, len(creds))
	i := 0
	for k := range creds {
		profileNames[i] = k
		i++
	}
	return profileNames
}

func contains(array []string, target string) bool {
	for i := 0; i < len(array); i++ {
		if array[i] == target {
			return true
		}
	}
	return false
}

func validateProfile(configuredProfiles []string, profile string) bool {
	if contains(configuredProfiles, profile) {
		return true
	}
	return false
}

func dedupeCreds(creds map[string]map[string]interface{}) map[string]map[string]interface{} {
	deduped := make(map[string]map[string]interface{})
	log("Deduping creds")
	for k := range creds {
		if creds[k]["aws_access_key_id"] != nil {
			if deduped[creds[k]["aws_access_key_id"].(string)] == nil {
				deduped[creds[k]["aws_access_key_id"].(string)] = make(map[string]interface{})
			}
			deduped[creds[k]["aws_access_key_id"].(string)]["aws_secret_access_key"] = creds[k]["aws_secret_access_key"].(string)
			if deduped[creds[k]["aws_access_key_id"].(string)]["profiles"] == nil {
				deduped[creds[k]["aws_access_key_id"].(string)]["profiles"] = make([]string, 0)
			}
			deduped[creds[k]["aws_access_key_id"].(string)]["profiles"] = append(deduped[creds[k]["aws_access_key_id"].(string)]["profiles"].([]string), k)
			deduped[creds[k]["aws_access_key_id"].(string)]["rolled"] = false
		}
	}
	return deduped
}

func main() {
	// parse command line flags
	profileFlag := flag.String("profile", "default", "AWS profile for which to rotate credentials. Use comma-delimited string to rotate multiple profiles. To rotate all profiles pass 'all'")
	awsCredsFileFlag := flag.String("config-dir", "~/.aws/", "Path for AWS CLI config files.")
	accountIdsFlag := flag.String("account-ids", "false", "AWS Account IDs for which to allow rotation of credentials. Use comma-delimited string to rotate credentials for multiple AWS accounts.")
	loggingValue := flag.Bool("debug", false, "Turn on debug output")
	flag.Parse()
	loggingEnabled = *loggingValue
	// loggingEnabled = true
	var accountIds []string
	if *accountIdsFlag != "false" {
		accountIds = strings.Split(*accountIdsFlag, ",")
	}
	log(accountIds)
	profiles := strings.Split(*profileFlag, ",")
	log(profiles)
	awsCredsFile := *awsCredsFileFlag
	log(awsCredsFile)

	// get current user
	user, err := user.Current()
	check(err)
	log(user)

	// find existing AWS config files
	credsFilePath := strings.Replace(awsCredsFile, "~", user.HomeDir, 1)
	log(credsFilePath)
	credsFiles := findCreds(credsFilePath)
	log(credsFiles)

	// back up existing config
	backup(credsFiles)

	// read current config
	creds := readCreds(credsFiles)
	log(creds)

	dedupedCreds := dedupeCreds(creds)
	log(dedupedCreds)

	configuredProfileNames := getProfileNames(creds)
	log(configuredProfileNames)

	if len(profiles) == 1 && profiles[0] == "all" {
		profiles = configuredProfileNames
	}

	oldKeyIds := make(map[string]string)

	for i := 0; i < len(profiles); i++ {
		profile := profiles[i]
		if creds[profile]["aws_access_key_id"] != nil {
			oldKeyIds[profile] = creds[profile]["aws_access_key_id"].(string)
		} else {
			oldKeyIds[profile] = ""
		}
		if validateProfile(configuredProfileNames, profile) {
			sess := awsSession(profile)
			log(validateSession(sess, accountIds))
			checkCreds(sess)
		} else {
			log("Invalid profile " + profile + ", skipping.")
		}
	}

	// iterate through target profiles
	for i := 0; i < len(profiles); i++ {
		profile := profiles[i]
		oldKeyID := oldKeyIds[profile]
		if validateProfile(configuredProfileNames, profile) {
			// get aws sdk session
			sess := awsSession(profile)

			if validateSession(sess, accountIds) &&
				contains(dedupedCreds[oldKeyID]["profiles"].([]string), profile) &&
				dedupedCreds[oldKeyID]["rolled"].(bool) == false {
				log("Rotating creds for profile " + profile)
				// rotate credentials
				newCreds := getNewCreds(sess)
				// set new credentials for config object in memory
				dedupedCreds[oldKeyID]["rolled"] = true
				for i := range dedupedCreds[oldKeyID]["profiles"].([]string) {
					if creds[dedupedCreds[oldKeyID]["profiles"].([]string)[i]]["aws_access_key_id"] != nil {
						log("Updating creds in map")
						creds[dedupedCreds[oldKeyID]["profiles"].([]string)[i]]["aws_access_key_id"] = *newCreds.AccessKeyId
						creds[dedupedCreds[oldKeyID]["profiles"].([]string)[i]]["aws_secret_access_key"] = *newCreds.SecretAccessKey
					}
				}
				print("Successfully rolled creds for " + profile)
			}
		}
	}

	// write config object to disk
	writeCreds(credsFilePath, creds)
}
