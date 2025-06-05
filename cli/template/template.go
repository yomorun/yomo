package template

import (
	"embed"
	"errors"
	"os"
	"slices"
	"strings"
)

//go:embed go
//go:embed node
var fs embed.FS

var (
	ErrUnsupportedSfnType   = errors.New("unsupported sfn type")
	ErrorUnsupportedRuntime = errors.New("unsupported runtime")
	ErrUnsupportedTest      = errors.New("unsupported test")
	ErrUnsupportedFeature   = errors.New("unsupported feature")
)

var (
	SupportedSfnTypes = []string{"llm", "normal"}
	SupportedRuntimes = []string{"node", "go"}
)

// get template content
func GetContent(command string, sfnType string, runtime string, isTest bool) ([]byte, error) {
	name, err := getTemplateFileName(command, sfnType, runtime, isTest)
	if err != nil {
		return nil, err
	}
	f, err := fs.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			if isTest {
				return nil, ErrUnsupportedTest
			}
			return nil, err
		}
		return nil, err
	}
	defer f.Close()
	_, err = f.Stat()
	if err != nil {
		return nil, err
	}

	return fs.ReadFile(name)
}

func getTemplateFileName(command string, sfnType string, runtime string, isTest bool) (string, error) {
	if command == "" {
		command = "init"
	}
	sfnType, err := validateSfnType(sfnType)
	if err != nil {
		return "", err
	}
	runtime, err = validateRuntime(runtime)
	if err != nil {
		return "", err
	}
	if runtime == "node" && sfnType == "normal" {
		return "", errors.New("language node (-l node) only support type llm (-t llm)")
	}

	sb := new(strings.Builder)
	sb.WriteString(runtime)
	sb.WriteString("/")
	sb.WriteString(command)
	sb.WriteString("_")
	sb.WriteString(sfnType)
	if isTest {
		sb.WriteString("_test")
	}
	sb.WriteString(".tmpl")

	// validate the path exists
	name := sb.String()

	return name, nil
}

func validateSfnType(sfnType string) (string, error) {
	if sfnType == "" {
		// default sfn type
		return "llm", nil
	}
	if slices.Contains(SupportedSfnTypes, sfnType) {
		return sfnType, nil
	}
	return sfnType, ErrUnsupportedSfnType
}

func validateRuntime(runtime string) (string, error) {
	if runtime == "" {
		// default lang
		return "node", nil
	}
	if slices.Contains(SupportedRuntimes, runtime) {
		return runtime, nil
	}
	return runtime, ErrorUnsupportedRuntime
}
