// Package runner exposes the APIs used by God for deploy and manage services in
// the Go ecosystem.
package runner

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/pioz/god/sshcmd"
	"gopkg.in/yaml.v3"
)

// Conf holds a service configuration.
type Conf struct {
	User           string `yaml:"user"`
	Host           string `yaml:"host"`
	Port           string `yaml:"port"`
	PrivateKeyPath string `yaml:"private_key_path"`

	GoExecPath     string `yaml:"go_exec_path"`
	GoBinDirectory string `yaml:"go_bin_directory"`
	GoInstall      string `yaml:"go_install"`

	GoPrivate     string `yaml:"go_private"`
	NetrcMachine  string `yaml:"netrc_machine"`
	NetrcLogin    string `yaml:"netrc_login"`
	NetrcPassword string `yaml:"netrc_password"`

	SystemdPath              string `yaml:"systemd_path"`
	SystemdServicesDirectory string `yaml:"systemd_services_directory"`
	SystemdLingerDirectory   string `yaml:"systemd_linger_directory"`

	ExecStart             string `yaml:"exec_start"`
	WorkingDirectory      string `yaml:"working_directory"`
	Environment           string `yaml:"environment"`
	LogPath               string `yaml:"log_path"`
	RunAfterService       string `yaml:"run_after_service"`
	StartLimitBurst       int    `yaml:"start_limit_burst"`
	StartLimitIntervalSec int    `yaml:"start_limit_interval_sec"`
	RestartSec            int    `yaml:"restart_sec"`

	CopyFiles []string `yaml:"copy_files"`

	Ignore bool `yaml:"ignore"`
}

type Runner struct {
	QuietMode    bool
	confFilePath string
	conf         map[string]*Conf
	services     map[string]Service
	mu           sync.Mutex
	output       chan message
	quit         chan struct{}
}

// MakeRunner loads the configuration from confFilePath and returns an
// initialized Runner.
func MakeRunner(confFilePath string) (*Runner, error) {
	runner := &Runner{
		confFilePath: confFilePath,
		services:     make(map[string]Service),
		output:       make(chan message),
		quit:         make(chan struct{}),
	}
	conf, err := readConf(confFilePath)
	if err != nil {
		return nil, err
	}
	runner.conf = conf
	return runner, nil
}

// GetServiceNames returns a slice with all not ignored services found in the
// configuration file.
func (r *Runner) GetServiceNames() []string {
	var names []string
	for key, value := range r.conf {
		if !value.Ignore {
			names = append(names, key)
		}
	}
	return names
}

// MakeService makes a new Service using the configuration under serviceName key
// in the configuration file.
func (r *Runner) MakeService(serviceName string) (Service, error) {
	// Fetch service from cache
	s, found := r.services[serviceName]
	if found {
		return s, nil
	}

	// Fetch service configuration
	conf, found := r.conf[serviceName]
	if !found {
		err := fmt.Errorf("configuration for service `%s` was not found. Please add service configuration in `%s` file", serviceName, r.confFilePath)
		return Service{}, err
	}

	// Validate configuration
	err := r.validateConf(conf)
	if err != nil {
		return Service{}, err
	}

	// Set SSH connection default configuration for missing values
	if conf.User == "" {
		currentUser, err := user.Current()
		if err == nil {
			conf.User = currentUser.Username
		}
	}
	if conf.Port == "" {
		conf.Port = "22"
	}
	if conf.PrivateKeyPath == "" {
		conf.PrivateKeyPath = filepath.Join(os.Getenv("HOME"), "/.ssh/id_rsa")
	}

	// Create SSH client
	client, err := sshcmd.MakeClient(conf.User, conf.Host, conf.Port, conf.PrivateKeyPath)
	if err != nil {
		return Service{}, err
	}

	// Connect the client
	err = client.Connect()
	if err != nil {
		return Service{}, err
	}

	// Create the service
	service := Service{Name: serviceName, Conf: conf, client: client, runner: r}

	// Find remote host working directory
	pwd, err := service.Exec("pwd")
	if err != nil {
		return Service{}, err
	}
	service.remoteHomeDir = pwd

	// Set default configuration for missing values

	// Go conf
	if conf.GoBinDirectory == "" {
		conf.GoBinDirectory, err = service.Exec("go env GOBIN")
		if err != nil {
			conf.GoBinDirectory = ""
		}
	}
	if conf.GoBinDirectory == "" {
		conf.GoBinDirectory, err = service.Exec("mise exec -- go env GOBIN")
		if err != nil {
			conf.GoBinDirectory = ""
		}
	}
	if conf.GoBinDirectory == "" {
		return Service{}, fmt.Errorf("$GOBIN environment variable is not set on the remote host: please set the $GOBIN env variable on the remote host or add `go_bin_directory: <path>` in `%s` file", r.confFilePath)
	}
	if conf.GoExecPath == "" {
		conf.GoExecPath, err = service.Exec("which go")
		if err != nil {
			conf.GoExecPath = ""
		}
	}
	if conf.GoExecPath == "" {
		conf.GoExecPath, err = service.Exec("mise exec -- which go")
		if err != nil {
			conf.GoExecPath = ""
		}
	}
	if conf.GoExecPath == "" {
		conf.GoExecPath = filepath.Join(conf.GoBinDirectory, "go")
	}

	// Systemd conf
	if conf.SystemdPath == "" {
		conf.SystemdPath = "systemd"
	}
	if conf.SystemdServicesDirectory == "" {
		conf.SystemdServicesDirectory = filepath.Join(pwd, ".config/systemd/user")
	}
	if conf.SystemdLingerDirectory == "" {
		conf.SystemdLingerDirectory = "/var/lib/systemd/linger"
	}

	// Service conf
	if conf.ExecStart == "" {
		exec := getExec(conf.GoInstall)
		if exec != "" {
			conf.ExecStart = filepath.Join(conf.GoBinDirectory, exec)
		}
	}
	if conf.WorkingDirectory == "" {
		conf.WorkingDirectory = pwd
	}
	// Save cache
	r.mu.Lock()
	r.services[serviceName] = service
	r.mu.Unlock()

	return service, nil
}

// StartPrintOutput starts a go routine that read messages from runner channel
// and prints them.
func (runner *Runner) StartPrintOutput(services []string) {
	width := 0
	for _, serviceName := range services {
		if len(serviceName) > width {
			width = len(serviceName)
		}
	}
	for {
		select {
		case message := <-runner.output:
			if !runner.QuietMode || message.status == MessageError {
				message.print(width)
			}
		case <-runner.quit:
			return
		}
	}
}

// StopPrintOutput stop the go routine started with StartPrintOutput.
func (runner *Runner) StopPrintOutput() {
	runner.quit <- struct{}{}
}

// SendMessage writes a message in the runner channel that can be captured and
// printed by the go routine started with StartPrintOutput.
func (runner *Runner) SendMessage(serviceName, text string, status MessageStatus) {
	runner.output <- message{
		serviceName: serviceName,
		text:        text,
		status:      status,
	}
}

// Private functions

func readConf(filename string) (map[string]*Conf, error) {
	conf := make(map[string]*Conf)

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(buf, conf)
	if err != nil {
		return nil, err
	}

	loadConfFromEnv(conf)

	return conf, nil
}

func loadConfFromEnv(conf map[string]*Conf) {
	for serviceName, value := range conf {
		reflectValue := reflect.ValueOf(value).Elem()
		for i := 0; i < reflectValue.NumField(); i++ {
			fieldReflectType := reflectValue.Type().Field(i)
			yamlTagValue := fieldReflectType.Tag.Get("yaml")
			if yamlTagValue != "" {
				envValue := os.Getenv(fmt.Sprintf("%s_%s", serviceNameToEnvName(serviceName), strings.ToUpper(yamlTagValue)))
				if envValue != "" {
					fieldValue := reflectValue.Field(i)
					switch fieldValue.Type().Name() {
					case "int":
						i, err := strconv.Atoi(envValue)
						if err == nil {
							fieldValue.Set(reflect.ValueOf(i))
						}
					case "string":
						fieldValue.Set(reflect.ValueOf(envValue))
					}
				}
			}
		}
	}
}

func (r *Runner) validateConf(conf *Conf) error {
	if conf.Host == "" {
		return fmt.Errorf("required configuration `host` value is missing: please add `host: <hostname>` in `%s` file", r.confFilePath)
	}
	if conf.GoInstall == "" {
		return fmt.Errorf("required configuration `go_install` value is missing: please add `go_install: <package>` in `%s` file", r.confFilePath)
	}
	return nil
}

var packageRegExp = regexp.MustCompile(`\/?([-_\w]+)@.*`)

func getExec(packageName string) string {
	match := packageRegExp.FindStringSubmatch(packageName)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func serviceNameToEnvName(serviceName string) string {
	if len(serviceName) == 0 {
		return ""
	}
	trim := func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		}
		return '_'
	}
	result := strings.ToUpper(strings.Map(trim, serviceName))
	if result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}
	return result
}
