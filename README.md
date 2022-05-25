# God - Go-daemons

God (go-daemons) is a tool to deploy and manage daemons in the GO ecosystem on GNU/Linux machines
using [Systemd](https://www.freedesktop.org/wiki/Software/systemd/).

God installs your go binary in the remote machine (server) using `go install`,
create the systemd unit file (.service file) and allows you to
start/stop/restart the process.

## Use case

I often write small micro-services or web-services in GO, and I need to deploy
the service on a server in an easy and fast way, without bothering! Usually I
compile the executable for the server architecture, log in via SSH on the
server, copy the binary, create the configuration file for the new systemd
service, install it and start the process. A series of commands that are always
the same but that I forget every time and have to look in my notes somewhere.
And that's not all: when I make a change to my program, then I have to recompile
it, log in via SSH on the server, copy the binary and restart the service. A
real waste of time!

So I decided to write God, a tool written in GO. With God I can quickly and
easily deploy and manage one or more services written in GO comfortably from my
laptop, without logging into the server via SSH using a simple
[YAML](https://yaml.org/) configuration file.

## Install God

```
go install github.com/pioz/god@latest
god -h
```

## Usage

📒 **To read the list of all commands and options run `god -h`**.

God uses a simple YAML file like this one to understand what to do:

```yaml
my_service_name1:
  user: pioz
  host: 119.178.21.21
  go_install: github.com/pioz/go_hello_world_server@latest
my_service_name2:
  user: pioz
  host: 119.178.21.22
  go_install: github.com/pioz/go_hello_world_server2@latest
```

You can install a new service on a remote server with the following command:

```
god -f conf/file.yml install my_service_name1
```

If you omit the `-f` option, God will try to find the conf file in `.god.yml`
path.

Now, what happens?

God will try to connect via SSH to the server `119.178.21.21` with the user
`pioz` on the default SSH port 22 using the private key stored locally in
`~/.ssh/id_rsa`. Currently, only authentication via private key is supported and
authentication via plain password is not planned. Then perform this sequence of
commands:

1. Check if GO is installed on the remote host
2. Check if Systemd is installed on the remote host
3. Check if the user `pioz` is in the [lingering list](https://www.freedesktop.org/software/systemd/man/loginctl.html)
4. Install the GO package `github.com/pioz/go_hello_world_server@latest` in `$GOBIN` (default `~/go/bin/`)
5. Create the systemd unit service file in `~/.config/systemd/user/`
6. Reload systemd daemon with `systemctl --user daemon-reload`
7. Enable the new service with `systemctl --user enable my_service_name1`

The systemd unit service file will be saved in
`~/.config/systemd/user/my_service_name1.service` and its content is
something like this:

```
[Unit]
Description=my_service_name1

[Service]
Type=simple
Restart=always
WorkingDirectory=/home/pioz
ExecStart=/home/pioz/go/bin/go_hello_world_server

[Install]
WantedBy=default.target
```

Now you can start the service with

```
god start my_service_name1
```

that will run in the remote host

```
systemctl --user start my_service_name1
```

If you want rollback and clean your server you can uninstall the service with
`god uninstall my_service_name1`.

### Install from private repository

If your Go service package is located in a private repository, God allows the
remote server to access the repository prepending in the remote
[`~/.netrc`](https://www.gnu.org/software/inetutils/manual/html_node/The-_002enetrc-file.html)
file the login information that you can specify on the God YAML configuration
file (read `god -h` for more details). An example here:

```yaml
my_private_service:
  user: pioz
  host: 119.178.21.21
  go_install: github.com/pioz/go_hello_world_server_private@latest
  # Private repo access
  go_private: github.com/pioz/go_hello_world_server_private
  netrc_machine: github.com
  netrc_login: githublogin@gmail.com
  netrc_password: youRgithubAcce$$tok3n
```

### Manage multiple services at the same time

If you do not specify a service name all services defined in the YAML file will
be taken into consideration, for example if you run `god restart` it will be
restart all services defined in the YAML file in parallel. 🤩

This is really useful if your infrastructure is made by many microservices.

### -help

```
god -h
Usage: god [OPTIONS...] {COMMAND} ...
  -f string
    	Configuration YAML file path (default ".god.yml")
  -h	Print this help
  -q	Disable printing

Commands:
After each command you can specify one or more services. If you do not specify any, all services in the YAML
configuration file will be selected.

install SERVICE...            Install one or more services on the remote host.
uninstall SERVICE...          Uninstall one or more services on the remote host.
start SERVICE...              Start one or more services.
stop SERVICE...               Stop one or more services.
restart SERVICE...            Restart one or more services.
status SERVICE...             Show runtime status of one or more services.
show-service SERVICE...       Print systemd unit service file of one or more services.

Configuration YAML file options:
user                          User to log in with on the remote machine. (default current user)
host                          Hostname to log in for executing commands on the remote host. (required)
port                          Port to connect to on the remote host. (default 22)
private_key_path              Local path of the private key used to authenticate on the remote host. (default
                              '~/.ssh/id_rsa')
go_exec_path                  Remote path of the GO binary executable. (default '/usr/local/go/bin/go')
go_bin_directory              The directory where 'go install' will install the service executable. (default
                              '~/go/bin/')
go_install                    Go package to install on the remote host. Package path must refer to main packages and
                              must have the version suffix, ex: @latest. (required)
go_private                    Set GOPRIVATE environment variable to be used when run 'go install' to install from
                              private sources.
netrc_machine                 Add in remote .netrc file the machine name to be used to access private repository.
netrc_login                   Add in remote .netrc file the login name to be used to access private repository.
netrc_password                Add in remote .netrc file the password or access token to be used to access private
                              repository.
systemd_path                  Remote path of systemd binary executable. (default 'systemd')
systemd_services_directory    Remote directory where to save user instance systemd unit service configuration file.
                              (default '~/.config/systemd/user/')
systemd_linger_dir            Remote directory where to find the lingering user list. If lingering is enabled for a
                              specific user, a user manager is spawned for the user at boot and kept around after
                              logouts. (default '/var/lib/systemd/linger/')
exec_start                    Command with its arguments that are executed when this service is started.
working_directory             Sets the remote working directory for executed processes. (default: '~/')
environment                   Sets environment variables for executed process. Takes a space-separated list of variable
                              assignments.
log_path                      Sets the remote file path where executed processes will redirect its standard output and
                              standard error.
run_after_service             Ensures that the service is started after the listed unit finished starting up.
start_limit_burst             Configure service start rate limiting. Services which are started more than burst times
                              within an interval time interval are not permitted to start any more. Use
                              'start_limit_interval_sec' to configure the checking interval.
start_limit_interval_sec      Configure the checking interval used by 'start_limit_burst'.
restart_sec                   Configures the time to sleep before restarting a service. Takes a unit-less value in
                              seconds.
ignore                        If a command is called without any service name, all services in the YAML configuration
                              file will be selected, except those with ignore set to true. (default false)
```

## Contributing

Bug reports and pull requests are welcome on GitHub at https://github.com/pioz/god/issues.

## License

The package is available as open source under the terms of the [MIT License](http://opensource.org/licenses/MIT).
