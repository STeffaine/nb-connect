package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/steffaine/nb-connect/internal/cache"
	"github.com/steffaine/nb-connect/internal/config"
	"github.com/steffaine/nb-connect/internal/connector"
	"github.com/steffaine/nb-connect/internal/launcher"
	"github.com/steffaine/nb-connect/internal/netbox"
)

type dependencies struct {
	loadConfig      func(string) (config.Config, error)
	loadCredentials func(string) (config.Credentials, error)
	defaultConfig   func() (string, error)
	defaultCache    func() (string, error)
	newClient       func(string, string) (*netbox.Client, error)
	now             func() time.Time
}

func NewRootCommand() *cobra.Command {
	return newRootCommand(dependencies{
		loadConfig: config.Load, loadCredentials: config.LoadCredentials,
		defaultConfig: config.DefaultConfigPath, defaultCache: cache.DefaultPath,
		newClient: func(url, token string) (*netbox.Client, error) { return netbox.NewClient(url, token, nil) },
		now:       time.Now,
	})
}

func newRootCommand(deps dependencies) *cobra.Command {
	var configPath string
	var credentialsPath string
	var cachePath string
	var debugAPI bool
	var connectDebugAPI bool
	var serverName string
	var target string
	var serviceName string
	var endpoint string
	var refresh bool
	var dryRun bool

	root := &cobra.Command{Use: "nbcon", Short: "Launch services discovered from NetBox", SilenceUsage: true}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to public configuration")
	root.PersistentFlags().StringVar(&credentialsPath, "credentials", "", "path to NetBox credentials")
	root.PersistentFlags().StringVar(&cachePath, "cache", "", "path to service cache")

	resolvePaths := func() (string, string, string, error) {
		if configPath == "" {
			path, err := deps.defaultConfig()
			if err != nil {
				return "", "", "", err
			}
			configPath = path
		}
		if credentialsPath == "" {
			credentialsPath = config.DefaultCredentialsPath(configPath)
		}
		if cachePath == "" {
			path, err := deps.defaultCache()
			if err != nil {
				return "", "", "", err
			}
			cachePath = path
		}
		return configPath, credentialsPath, cachePath, nil
	}

	refreshServices := func(ctx context.Context, debugOutput io.Writer) ([]netbox.Service, error) {
		configurationPath, credentialPath, serviceCachePath, err := resolvePaths()
		if err != nil {
			return nil, err
		}
		configuration, err := deps.loadConfig(configurationPath)
		if err != nil {
			return nil, err
		}
		credentials, err := deps.loadCredentials(credentialPath)
		if err != nil {
			return nil, err
		}
		var services []netbox.Service
		for _, server := range configuration.NetBox.Servers {
			token, err := credentials.TokenFor(server.Name)
			if err != nil {
				return nil, err
			}
			client, err := deps.newClient(server.URL, token)
			if err != nil {
				return nil, err
			}
			if debugOutput != nil {
				client.SetDebugOutput(debugOutput)
			}
			if err := client.Validate(ctx); err != nil {
				return nil, fmt.Errorf("validate NetBox server %q: %w", server.Name, err)
			}
			serverServices, err := client.Services(ctx)
			if err != nil {
				return nil, fmt.Errorf("query NetBox server %q: %w", server.Name, err)
			}
			for index := range serverServices {
				serverServices[index].Server = server.Name
			}
			services = append(services, serverServices...)
		}
		services = filterServices(services, configuration.Services.Enabled)
		if err := (cache.Store{Path: serviceCachePath}).Write(services, deps.now()); err != nil {
			return nil, err
		}
		return services, nil
	}

	syncServices := func(command *cobra.Command, debug bool) error {
		var debugOutput io.Writer
		if debug {
			debugOutput = command.ErrOrStderr()
		}
		services, err := refreshServices(command.Context(), debugOutput)
		if err != nil {
			return err
		}
		fmt.Fprintln(command.OutOrStdout(), "Connected to NetBox")
		fmt.Fprintf(command.OutOrStdout(), "Found %d services\nCache updated\n", len(services))
		return nil
	}

	syncCommand := &cobra.Command{
		Use: "sync", Short: "Synchronize services from NetBox",
		RunE: func(command *cobra.Command, args []string) error {
			return syncServices(command, debugAPI)
		},
	}
	syncCommand.Flags().BoolVar(&debugAPI, "debug-api", false, "print NetBox API requests and response bodies to stderr")
	root.AddCommand(syncCommand)

	connectCommand := &cobra.Command{
		Use: "connect", Short: "Connect to a cached service",
		RunE: func(command *cobra.Command, args []string) error {
			configurationPath, _, serviceCachePath, err := resolvePaths()
			if err != nil {
				return err
			}
			configuration, err := deps.loadConfig(configurationPath)
			if err != nil {
				return err
			}
			if refresh {
				if err := syncServices(command, connectDebugAPI); err != nil {
					return err
				}
			}
			snapshot, err := (cache.Store{Path: serviceCachePath}).Read()
			if err != nil {
				return err
			}

			services := filterServicesByServer(snapshot.Services, serverName)
			selection, err := selectService(command.Context(), services, target, serviceName, endpoint, configuration.Ping.Count, func(ctx context.Context) ([]netbox.Service, error) {
				return refreshServices(ctx, nil)
			})
			if err != nil {
				return ignoreSelectionCancellation(err)
			}
			connectionCommand, err := connectionCommandFor(selection, configuration)
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintln(command.OutOrStdout(), formatCommand(connectionCommand))
				return nil
			}

			process := exec.CommandContext(command.Context(), connectionCommand.Name, connectionCommand.Args...)
			process.Stdin = command.InOrStdin()
			process.Stdout = command.OutOrStdout()
			process.Stderr = command.ErrOrStderr()
			return process.Run()
		},
	}
	connectCommand.Flags().StringVar(&serverName, "server", "", "NetBox server name")
	connectCommand.Flags().StringVar(&target, "target", "", "target device or virtual machine name")
	connectCommand.Flags().StringVar(&serviceName, "service", "", "NetBox service name")
	connectCommand.Flags().StringVar(&endpoint, "endpoint", "", "endpoint in host:port form")
	connectCommand.Flags().BoolVar(&refresh, "refresh", false, "synchronize from NetBox before connecting")
	connectCommand.Flags().BoolVar(&dryRun, "dry-run", false, "print the SSH command without executing it")
	connectCommand.Flags().BoolVar(&connectDebugAPI, "debug-api", false, "print NetBox API requests and response bodies when used with --refresh")
	root.AddCommand(connectCommand)
	root.RunE = connectCommand.RunE

	root.AddCommand(&cobra.Command{
		Use: "list", Short: "List services from the local cache",
		RunE: func(command *cobra.Command, args []string) error {
			_, _, serviceCachePath, err := resolvePaths()
			if err != nil {
				return err
			}
			snapshot, err := (cache.Store{Path: serviceCachePath}).Read()
			if err != nil {
				return err
			}
			writeServices(command.OutOrStdout(), snapshot.Services)
			return nil
		},
	})

	root.SetOut(os.Stdout)
	return root
}

func ignoreSelectionCancellation(err error) error {
	if errors.Is(err, launcher.ErrSelectionCancelled) {
		return nil
	}
	return err
}

func connectionCommandFor(selection launcher.Selection, configuration config.Config) (connector.Command, error) {
	if strings.EqualFold(strings.TrimSpace(selection.Service.Name), "telnet") {
		return connector.Telnet(selection.Service, selection.Endpoint)
	}
	identityFile := configuration.SSH.Keys[configuration.SSH.DefaultUser].IdentityFile
	return connector.SSH(selection.Service, selection.Endpoint, configuration.SSH.DefaultUser, identityFile)
}

func selectService(ctx context.Context, services []netbox.Service, target, serviceName, endpoint string, pingCount int, syncServices launcher.SyncServices) (launcher.Selection, error) {
	if target == "" && serviceName == "" && endpoint == "" {
		return launcher.Select(ctx, services, pingCount, syncServices)
	}
	if target == "" || serviceName == "" {
		return launcher.Selection{}, fmt.Errorf("--target and --service must be used together")
	}
	var matches []netbox.Service
	for _, service := range services {
		if strings.EqualFold(service.TargetName(), target) && strings.EqualFold(service.Name, serviceName) {
			matches = append(matches, service)
		}
	}
	if len(matches) == 0 {
		return launcher.Selection{}, fmt.Errorf("no cached %q service found for target %q", serviceName, target)
	}
	if len(matches) > 1 {
		return launcher.Selection{}, fmt.Errorf("multiple cached %q services found for target %q", serviceName, target)
	}
	return launcher.Selection{Service: matches[0], Endpoint: endpoint}, nil
}

func formatCommand(command connector.Command) string {
	parts := make([]string, 0, len(command.Args)+1)
	parts = append(parts, command.Name)
	for _, argument := range command.Args {
		parts = append(parts, fmt.Sprintf("%q", argument))
	}
	return strings.Join(parts, " ")
}

func filterServices(services []netbox.Service, enabledNames []string) []netbox.Service {
	enabled := make(map[string]struct{}, len(enabledNames))
	for _, name := range enabledNames {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			enabled[name] = struct{}{}
		}
	}

	filtered := make([]netbox.Service, 0, len(services))
	for _, service := range services {
		if _, ok := enabled[strings.ToLower(strings.TrimSpace(service.Name))]; ok {
			filtered = append(filtered, service)
		}
	}
	return filtered
}

func filterServicesByServer(services []netbox.Service, serverName string) []netbox.Service {
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return services
	}
	filtered := make([]netbox.Service, 0, len(services))
	for _, service := range services {
		if strings.EqualFold(service.Server, serverName) {
			filtered = append(filtered, service)
		}
	}
	return filtered
}

func writeServices(output io.Writer, services []netbox.Service) {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SERVER\tTARGET\tSERVICE\tENDPOINTS\tROLE\tTENANT\tSTATUS")
	for _, service := range services {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", service.Server, service.TargetName(), service.Name, join(service.Endpoints()), service.Role, service.Tenant, service.Status)
	}
	writer.Flush()
}

func join(items []string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for _, item := range items[1:] {
		result += "," + item
	}
	return result
}

func Run(ctx context.Context, args []string, output io.Writer) error {
	command := NewRootCommand()
	command.SetArgs(args)
	command.SetOut(output)
	return command.ExecuteContext(ctx)
}
