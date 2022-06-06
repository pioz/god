package runner

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pioz/god/sshcmd"
)

// Service represents a service that will be installed and launched on the
// remote machine.
type Service struct {
	// The service name (key in the configuration YAML file)
	Name string
	// Configuration under the key in the configuration YAML file
	Conf *Conf

	client        *sshcmd.Client
	runner        *Runner
	remoteHomeDir string
}

// Exec runs cmd on the remote host.
func (service *Service) Exec(cmd string) (string, error) {
	output, err := service.client.Exec(cmd)
	return strings.TrimSuffix(output, "\n"), err
}

// PrintExec runs cmd on the remote host and sends the output on the runner
// channel.
func (service *Service) PrintExec(cmd, errorMessage string) error {
	service.runner.SendMessage(service.Name, cmd, MessageNormal)
	output, err := service.Exec(cmd)
	if err != nil {
		if errorMessage == "" {
			errorMessage = output
		} else {
			errorMessage = fmt.Sprintf("%s: %s", errorMessage, output)
		}
		service.runner.SendMessage(service.Name, errorMessage, MessageError)
		return err
	} else {
		service.runner.SendMessage(service.Name, output, MessageSuccess)
		return nil
	}
}

// ParseCommand parses the cmd string replacing the variables with those present
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

// CopyFile copies the local file on the remote host to the remote
// workingDirectory. If the local file is a directory, create the directory on
// the remote host and recursively copy all files inside.
func (service *Service) CopyFile(path, workingDirectory string) error {
	err := service.client.ConnectSftpClient()
	if err != nil {
		return err
	}

	return service.client.WalkDir(path, workingDirectory, func(localPath, remotePath string, info fs.DirEntry, e error) error {
		if info.IsDir() {
			return service.client.SftClient.MkdirAll(remotePath)
		}
		srcFile, err := os.Open(localPath)
		if err != nil {
			return err
		}

		dstFile, err := service.client.SftClient.Create(remotePath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = dstFile.ReadFrom(srcFile)
		return err
	})
}

// DeleteFile deletes the file on the remote host relative to the remote
// workingDirectory.
func (service *Service) DeleteFile(path, workingDirectory string) error {
	var directories []string
	err := service.client.ConnectSftpClient()
	if err != nil {
		return err
	}
	err = service.client.WalkDir(path, workingDirectory, func(localPath, remotePath string, info fs.DirEntry, e error) error {
		if info.IsDir() {
			directories = append(directories, remotePath)
		} else {
			service.client.SftClient.Remove(remotePath)
		}
		return nil
	})
	for i := len(directories) - 1; i >= 0; i-- {
		service.client.SftClient.RemoveDirectory(directories[i])
	}
	return err
}

// CopyUnitServiceFile copies the systemd unit service file on the remote host.
func (service *Service) CopyUnitServiceFile() error {
	err := service.client.ConnectSftpClient()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	service.GenerateServiceFile(&buf)

	// Create the destination file
	filename := filepath.Join(service.Conf.SystemdServicesDirectory, fmt.Sprintf("%s.service", service.Name))
	dstFile, err := service.client.SftClient.Create(filename)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// write to file
	if _, err := dstFile.ReadFrom(&buf); err != nil {
		return err
	}
	return nil
}

// GenerateServiceFile generates the systemd unit service file using the service
// configuration.
func (service *Service) GenerateServiceFile(buf io.Writer) {
	tmpl, err := template.New("serviceFile").Parse(fmt.Sprintf(serviceTemplate, service.Name))
	if err != nil {
		panic(err)
	}
	tmpl.Execute(buf, service.Conf)
}

// DeleteDirIfEmpty deletes remote directory only if empty.
func (service *Service) DeleteDirIfEmpty(dirPath string) error {
	err := service.client.ConnectSftpClient()
	if err != nil {
		return err
	}
	return service.client.SftClient.Remove(dirPath)
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
