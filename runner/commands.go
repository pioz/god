package runner

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/exp/slices"
)

func (s *Service) CheckGo() error {
	errorMessage := fmt.Sprintf("couldn't find the `go` executable. Please install `go` or set the executable path in `%s` file using the `go_exec_path` variable", s.runner.confFilePath)
	cmd := s.ParseCommand("{{.GoExecPath}} version")
	return s.PrintExec(cmd, errorMessage)
}

func (s *Service) CheckSystemd() error {
	errorMessage := fmt.Sprintf("couldn't find the `systemd` executable. Please install `systemd` or set the executable path in `%s` file using the `systemd_path` variable", s.runner.confFilePath)
	cmd := s.ParseCommand("{{.SystemdPath}} --version")
	return s.PrintExec(cmd, errorMessage)
}

func (s *Service) CheckLingering() error {
	cmd := s.ParseCommand("ls {{.SystemdLingerDirectory}}")
	s.runner.SendMessage(s.Name, cmd, MessageNormal)
	output, err := s.Exec(cmd)
	if err != nil {
		s.runner.SendMessage(s.Name, err.Error(), MessageError)
		return err
	}
	if !slices.Contains(strings.Split(output, "\n"), s.Conf.User) {
		err = fmt.Errorf("user `%s` is not in the linger list. You can add it with the command `sudo loginctl enable-linger %s`", s.Conf.User, s.Conf.User)
		s.runner.SendMessage(s.Name, err.Error(), MessageError)
		return err
	}
	s.runner.SendMessage(s.Name, output, MessageSuccess)
	return nil
}

func (s *Service) CheckWorkingDir(createWorkingDirectory bool) error {
	cmd := s.ParseCommand("test -e {{.WorkingDirectory}}")
	s.runner.SendMessage(s.Name, cmd, MessageNormal)
	_, err := s.Exec(cmd)
	if err == nil {
		s.runner.SendMessage(s.Name, "", MessageSuccess)
		return nil
	}
	if !createWorkingDirectory {
		s.runner.SendMessage(s.Name, fmt.Sprintf("Service working directory '%s' does not exist on the remote host", s.Conf.WorkingDirectory), MessageError)
		return err
	}
	cmd = s.ParseCommand("mkdir -p {{.WorkingDirectory}}")
	s.runner.SendMessage(s.Name, cmd, MessageNormal)
	output, err := s.Exec(cmd)
	if err != nil {
		s.runner.SendMessage(s.Name, err.Error(), MessageError)
		return err
	}
	s.runner.SendMessage(s.Name, output, MessageSuccess)
	return nil
}

func (s *Service) AuthPrivateRepo() error {
	if s.Conf.GoPrivate != "" {
		s.runner.SendMessage(s.Name, "GO_PRIVATE found: edit .netrc file", MessageNormal)
		auth := fmt.Sprintf("machine %s login %s password %s", s.Conf.NetrcMachine, s.Conf.NetrcLogin, s.Conf.NetrcPassword)
		output, err := s.Exec("cat ~/.netrc")
		if !strings.Contains(output, auth) {
			var cmd string
			if err != nil {
				cmd = fmt.Sprintf("echo '%s' > ~/.netrc", auth)
			} else {
				cmd = fmt.Sprintf("echo '%s\n%s' > ~/.netrc", auth, output)
			}
			_, err := s.Exec(cmd)
			if err != nil {
				s.runner.SendMessage(s.Name, err.Error(), MessageError)
				return err
			}
		}
		s.runner.SendMessage(s.Name, "", MessageSuccess)
	}
	return nil
}

func (s *Service) InstallExecutable() error {
	var cmd string
	if s.Conf.GoPrivate != "" {
		cmd = s.ParseCommand("GOPRIVATE={{.GoPrivate}} {{.GoExecPath}} install {{.GoInstall}}")
	} else {
		cmd = s.ParseCommand("{{.GoExecPath}} install {{.GoInstall}}")
	}
	errorMessage := fmt.Sprintf("cannot install the package `%s`", s.Conf.GoInstall)
	s.runner.SendMessage(s.Name, cmd, MessageNormal)
	output, err := s.Exec(cmd)
	if err != nil {
		errorMessage = fmt.Sprintf("%s: %s", errorMessage, output)
		s.runner.SendMessage(s.Name, fmt.Sprintf("%s: %s", errorMessage, output), MessageError)
		return err
	}
	cmd = s.ParseCommand("file {{.ExecStart}}")
	errorMessage = fmt.Sprintf("couldn't find the `%s` executable", s.Conf.ExecStart)
	output, err = s.Exec(cmd)
	if err != nil {
		s.runner.SendMessage(s.Name, fmt.Sprintf("%s: %s", errorMessage, output), MessageError)
		return err
	}
	s.runner.SendMessage(s.Name, "Installed", MessageSuccess)
	return nil
}

func (s *Service) DeleteExecutable() error {
	errorMessage := fmt.Sprintf("cannot delete service binary file `%s`", s.Conf.ExecStart)
	cmd := s.ParseCommand("rm {{.ExecStart}}")
	return s.PrintExec(cmd, errorMessage)
}

func (s *Service) CreateServiceFile() error {
	message := fmt.Sprintf("Copy service file in `%s`", s.Conf.SystemdServicesDirectory)
	s.runner.SendMessage(s.Name, message, MessageNormal)
	err := s.CopyUnitServiceFile()
	if err != nil {
		s.runner.SendMessage(s.Name, err.Error(), MessageError)
		return err
	}
	s.runner.SendMessage(s.Name, "Copied", MessageSuccess)
	return nil
}

func (s *Service) CopyFiles() error {
	if len(s.Conf.CopyFiles) > 0 {
		s.runner.SendMessage(s.Name, "Copying files", MessageNormal)
		for _, path := range s.Conf.CopyFiles {
			err := s.CopyFile(path, s.Conf.WorkingDirectory)
			if err != nil {
				errorMessage := fmt.Sprintf("cannot copy file '%s': %s", path, err)
				s.runner.SendMessage(s.Name, errorMessage, MessageError)
				return err
			}
		}
		s.runner.SendMessage(s.Name, "All files copied", MessageSuccess)
	}
	return nil
}

func (s *Service) DeleteFiles(removeWorkingDirectory bool) error {
	if len(s.Conf.CopyFiles) > 0 {
		s.runner.SendMessage(s.Name, "Deleting files", MessageNormal)
		for _, path := range s.Conf.CopyFiles {
			err := s.DeleteFile(path, s.Conf.WorkingDirectory)
			if err != nil {
				errorMessage := fmt.Sprintf("cannot delete file '%s': %s", path, err)
				s.runner.SendMessage(s.Name, errorMessage, MessageWarning)
			}
		}
		s.runner.SendMessage(s.Name, "All files deleted", MessageSuccess)
	}
	if removeWorkingDirectory {
		if s.Conf.LogPath != "" {
			s.runner.SendMessage(s.Name, fmt.Sprintf("Deleting log file '%s'", s.Conf.LogPath), MessageNormal)
			err := s.client.ConnectSftpClient()
			if err != nil {
				s.runner.SendMessage(s.Name, fmt.Sprintf("Cannot delete log file '%s': %s", s.Conf.LogPath, err.Error()), MessageError)
			}
			err = s.client.SftClient.Remove(s.Conf.LogPath)
			if err != nil {
				s.runner.SendMessage(s.Name, fmt.Sprintf("Cannot delete log file '%s': %s", s.Conf.LogPath, err.Error()), MessageError)
			} else {
				s.runner.SendMessage(s.Name, "Deleted", MessageSuccess)
			}
		}
		if s.Conf.WorkingDirectory != s.remoteHomeDir {
			s.runner.SendMessage(s.Name, fmt.Sprintf("Deleting service working directory '%s'", s.Conf.WorkingDirectory), MessageNormal)
			err := s.DeleteDirIfEmpty(s.Conf.WorkingDirectory)
			if err, ok := err.(*sftp.StatusError); ok {
				switch err.Code {
				case 4: // sshFxFailure
					s.runner.SendMessage(s.Name, fmt.Sprintf("Cannot delete service working directory '%s': directory is not empty", s.Conf.WorkingDirectory), MessageError)
				default:
					s.runner.SendMessage(s.Name, fmt.Sprintf("Cannot delete service working directory '%s': %s", s.Conf.WorkingDirectory, err.Error()), MessageError)
				}
			}
			s.runner.SendMessage(s.Name, "Deleted", MessageSuccess)
		}
	}
	return nil
}

func (s *Service) ShowServiceFile() {
	var buf bytes.Buffer
	s.GenerateServiceFile(&buf)
	s.runner.SendMessage(s.Name, buf.String(), MessageNormal)
}

func (s *Service) DeleteServiceFile() error {
	filename := filepath.Join(s.Conf.SystemdServicesDirectory, fmt.Sprintf("%s.service", s.Name))
	errorMessage := fmt.Sprintf("cannot delete service file `%s`", filename)
	return s.PrintExec(fmt.Sprintf("rm %s", filename), errorMessage)
}

func (s *Service) ReloadDaemon() error {
	return s.PrintExec("systemctl --user daemon-reload", "couldn't reload systemd daemon")
}

func (s *Service) ResetFailedServices() error {
	return s.PrintExec("systemctl --user reset-failed", "couldn't reset failed systemd services")
}

func (s *Service) EnableService() error {
	return s.PrintExec(fmt.Sprintf("systemctl --user enable %s", s.Name), "couldn't enable systemd service")
}

func (s *Service) DisableService() error {
	return s.PrintExec(fmt.Sprintf("systemctl --user disable %s", s.Name), "couldn't disable systemd service")
}

func (s *Service) StartService() error {
	return s.PrintExec(fmt.Sprintf("systemctl --user start %s", s.Name), "couldn't start systemd service")
}

func (s *Service) StopService() error {
	return s.PrintExec(fmt.Sprintf("systemctl --user stop %s", s.Name), "couldn't stop systemd service")
}

func (s *Service) RestartService() error {
	return s.PrintExec(fmt.Sprintf("systemctl --user restart %s", s.Name), "couldn't restart systemd service")
}

func (s *Service) StatusService() error {
	return s.PrintExec(fmt.Sprintf("systemctl --user status %s", s.Name), "")
}

func (s *Service) Install(createWorkingDirectory bool) error {
	if err := s.CheckGo(); err != nil {
		return err
	}
	if err := s.CheckSystemd(); err != nil {
		return err
	}
	if err := s.CheckLingering(); err != nil {
		return err
	}
	if err := s.CheckWorkingDir(createWorkingDirectory); err != nil {
		return err
	}
	if err := s.AuthPrivateRepo(); err != nil {
		return err
	}
	if err := s.InstallExecutable(); err != nil {
		return err
	}
	if err := s.CopyFiles(); err != nil {
		return err
	}
	if err := s.CreateServiceFile(); err != nil {
		return err
	}
	if err := s.ReloadDaemon(); err != nil {
		return err
	}
	if err := s.EnableService(); err != nil {
		return err
	}
	return nil
}

func (s *Service) Uninstall(removeWorkingDirectory bool) {
	s.StopService()
	s.DisableService()
	s.DeleteServiceFile()
	s.ReloadDaemon()
	s.ResetFailedServices()
	s.DeleteExecutable()
	s.DeleteFiles(removeWorkingDirectory)
}
