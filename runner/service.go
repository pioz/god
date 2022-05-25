package runner

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"strings"

	"github.com/pioz/god/sshcmd"
	"github.com/pkg/sftp"
)

// Service represents a service that will be installed and launched on the
// remote machine.
type Service struct {
	// The service name (key in the configuration YAML file)
	Name string
	// Configuration under the key in the configuration YAML file
	Conf *Conf
	// SSH client
	Client *sshcmd.Client
	runner *Runner
}

// PrintExec run cmd on the remote host and send the output on the runner
// channel.
func (service *Service) PrintExec(cmd, errorMessage string) error {
	service.runner.SendMessage(service.Name, cmd, MessaggeNormal)
	output, err := service.Exec(cmd)
	if err != nil {
		if errorMessage == "" {
			errorMessage = output
		} else {
			errorMessage = fmt.Sprintf("%s: %s", errorMessage, output)
		}
		service.runner.SendMessage(service.Name, errorMessage, MessaggeError)
		return err
	} else {
		service.runner.SendMessage(service.Name, output, MessaggeSuccess)
		return nil
	}
}

// Exec run cmd on the remote host.
func (service *Service) Exec(cmd string) (string, error) {
	output, err := service.Client.Exec(cmd)
	return strings.TrimSuffix(output, "\n"), err
}

// ParseCommand parse the cmd string replacing the variables with those present
// in the configuration.
func (service *Service) ParseCommand(cmd string) string {
	tmpl, err := template.New("command").Parse(cmd)
	if err != nil {
		panic(err)
	}
	var parsedCommand bytes.Buffer
	tmpl.Execute(&parsedCommand, service.Conf)
	return parsedCommand.String()
}

// GenerateServiceFile generate the systemd unit service file using the service
// configuration.
func (service *Service) GenerateServiceFile(buf io.Writer) {
	tmpl, err := template.New("serviceFile").Parse(fmt.Sprintf(serviceTemplate, service.Name))
	if err != nil {
		panic(err)
	}
	tmpl.Execute(buf, service.Conf)
}

// CopyFile copy the systemd unit service file on the remote host.
func (service *Service) CopyFile(buf io.Reader) error {
	sftp, err := sftp.NewClient(service.Client.SshClient)
	if err != nil {
		return err
	}
	defer sftp.Close()

	// Create the destination file
	filename := filepath.Join(service.Conf.SystemdServicesDirectory, fmt.Sprintf("%s.service", service.Name))
	dstFile, err := sftp.Create(filename)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// write to file
	if _, err := dstFile.ReadFrom(buf); err != nil {
		return err
	}
	return nil
}

const serviceTemplate = `[Unit]
Description=%s
{{- if .RunAfterService}}
After={{.RunAfterService}}
{{- end}}
{{- if .StartLimitBurst}}
StartLimitBurst={{.StartLimitBurst}}
{{- end}}
{{- if .StartLimitIntervalSec}}
StartLimitIntervalSec={{.StartLimitIntervalSec}}
{{- end}}

[Service]
Type=simple
Restart=always
{{- if .RestartSec}}
RestartSec={{.RestartSec}}
{{- end}}
{{- if .Environment}}
Environment={{.Environment}}
{{- end}}
{{- if .LogPath}}
StandardOutput=append:{{.LogPath}}
{{- end}}
{{- if .LogPath}}
StandardError=append:{{.LogPath}}
{{- end}}
WorkingDirectory={{.WorkingDirectory}}
ExecStart={{.ExecStart}}

[Install]
WantedBy=default.target`
