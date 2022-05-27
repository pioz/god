package main

import (
	"flag"
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/pioz/god/runner"
	"golang.org/x/exp/slices"
)

var availableCommands = []string{"install", "uninstall", "start", "stop", "restart", "status", "show-service"}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [OPTIONS...] {COMMAND} ...\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintln(flag.CommandLine.Output(), lipgloss.NewStyle().Bold(true).Render("Commands:"))
		fmt.Fprintln(flag.CommandLine.Output(), lipgloss.NewStyle().Width(120).Render("After each command you can specify one or more services. If you do not specify any, all services in the YAML configuration file will be selected."))
		fmt.Fprintln(flag.CommandLine.Output())
		commands := [][]string{
			{"install SERVICE...", "Install one or more services on the remote host."},
			{"uninstall SERVICE...", "Uninstall one or more services on the remote host."},
			{"start SERVICE...", "Start one or more services."},
			{"stop SERVICE...", "Stop one or more services."},
			{"restart SERVICE...", "Restart one or more services."},
			{"status SERVICE...", "Show runtime status of one or more services."},
			{"show-service SERVICE...", "Print systemd unit service file of one or more services."},
		}
		for _, command := range commands {
			fmt.Fprintln(
				flag.CommandLine.Output(),
				lipgloss.JoinHorizontal(
					lipgloss.Top,
					lipgloss.NewStyle().Width(30).Render(command[0]),
					lipgloss.NewStyle().Width(90).Render(command[1]),
				),
			)
		}
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintln(flag.CommandLine.Output(), lipgloss.NewStyle().Bold(true).Render("Configuration YAML file options:"))
		confOptions := [][]string{
			{"user", "User to log in with on the remote machine. (default current user)"},
			{"host", "Hostname to log in for executing commands on the remote host. (required)"},
			{"port", "Port to connect to on the remote host. (default 22)"},
			{"private_key_path", "Local path of the private key used to authenticate on the remote host. (default '~/.ssh/id_rsa')"},
			{"go_exec_path", "Remote path of the Go binary executable. (default '/usr/local/go/bin/go')"},
			{"go_bin_directory", "The directory where 'go install' will install the service executable. (default '~/go/bin/')"},
			{"go_install", "Go package to install on the remote host. Package path must refer to main packages and must have the version suffix, ex: @latest. (required)"},
			{"go_private", "Set GOPRIVATE environment variable to be used when run 'go install' to install from private sources."},
			{"netrc_machine", "Add in remote .netrc file the machine name to be used to access private repository."},
			{"netrc_login", "Add in remote .netrc file the login name to be used to access private repository."},
			{"netrc_password", "Add in remote .netrc file the password or access token to be used to access private repository."},
			{"systemd_path", "Remote path of systemd binary executable. (default 'systemd')"},
			{"systemd_services_directory", "Remote directory where to save user instance systemd unit service configuration file. (default '~/.config/systemd/user/')"},
			{"systemd_linger_dir", "Remote directory where to find the lingering user list. If lingering is enabled for a specific user, a user manager is spawned for the user at boot and kept around after logouts. (default '/var/lib/systemd/linger/')"},
			{"exec_start", "Command with its arguments that are executed when this service is started."},
			{"working_directory", "Sets the remote working directory for executed processes. (default: '~/')"},
			{"environment", "Sets environment variables for executed process. Takes a space-separated list of variable assignments."},
			{"log_path", "Sets the remote file path where executed processes will redirect its standard output and standard error."},
			{"run_after_service", "Ensures that the service is started after the listed unit finished starting up."},
			{"start_limit_burst", "Configure service start rate limiting. Services which are started more than burst times within an interval time interval are not permitted to start any more. Use 'start_limit_interval_sec' to configure the checking interval."},
			{"start_limit_interval_sec", "Configure the checking interval used by 'start_limit_burst'."},
			{"restart_sec", "Configures the time to sleep before restarting a service. Takes a unit-less value in seconds."},
			{"ignore", "If a command is called without any service name, all services in the YAML configuration file will be selected, except those with ignore set to true. (default false)"},
		}
		for _, option := range confOptions {
			fmt.Fprintln(
				flag.CommandLine.Output(),
				lipgloss.JoinHorizontal(
					lipgloss.Top,
					lipgloss.NewStyle().Width(30).Render(option[0]),
					lipgloss.NewStyle().Width(90).Render(option[1]),
				),
			)
		}
	}
}

func main() {
	var help, quiet bool
	var confFilePath string
	flag.StringVar(&confFilePath, "f", ".god.yml", "Configuration YAML file path")
	flag.BoolVar(&quiet, "q", false, "Disable printing")
	flag.BoolVar(&help, "h", false, "Print this help")
	flag.Parse()
	if help {
		flag.Usage()
		os.Exit(0)
	}

	command := flag.Args()[0]
	services := flag.Args()[1:]

	if !slices.Contains(availableCommands, command) {
		fmt.Println(command)
		flag.Usage()
		os.Exit(1)
	}

	r, err := runner.MakeRunner(confFilePath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	r.QuietMode = quiet
	if len(services) == 0 {
		services = r.GetServiceNames()
	}
	go r.StartPrintOutput(services)
	defer r.StopPrintOutput()

	var wg sync.WaitGroup
	wg.Add(len(services))

	switch command {
	case "install":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.Install()
					}
				}(serviceName)
			}
		}
	case "uninstall":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.Uninstall()
					}
				}(serviceName)
			}
		}
	case "start":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.StartService()
					}
				}(serviceName)
			}
		}
	case "stop":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.StopService()
					}
				}(serviceName)
			}
		}
	case "restart":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.RestartService()
					}
				}(serviceName)
			}
		}
	case "status":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.StatusService()
					}
				}(serviceName)
			}
		}
	case "show-service":
		{
			for _, serviceName := range services {
				go func(serviceName string) {
					defer wg.Done()
					s, err := r.MakeService(serviceName)
					if err != nil {
						r.SendMessage(serviceName, err.Error(), runner.MessaggeError)
					} else {
						s.ShowServiceFile()
					}
				}(serviceName)
			}
		}
	}

	wg.Wait()
}
