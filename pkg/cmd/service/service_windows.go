package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	rdbg "runtime/debug"
	"time"

	"github.com/myopenfactory/client/pkg/client"
	cmdpkg "github.com/myopenfactory/client/pkg/cmd"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func init() {
	Command.PersistentFlags().String("name", "myOpenFactory Client", "name of the service")
	serviceRunCmd.Flags().Bool("debug", false, "debug windows service")
	serviceInstallCmd.Flags().String("logon", "", "windows logon name for the service")
	serviceInstallCmd.Flags().String("password", "", "windows logon password for the service")

	viper.BindPFlag("service.name", Command.PersistentFlags().Lookup("name"))
	viper.BindPFlag("service.logon", serviceInstallCmd.Flags().Lookup("logon"))
	viper.BindPFlag("service.password", serviceInstallCmd.Flags().Lookup("password"))
	viper.BindPFlag("service.debug", serviceRunCmd.Flags().Lookup("debug"))

	Command.AddCommand(serviceInstallCmd)
	Command.AddCommand(serviceUninstallCmd)
	Command.AddCommand(serviceRunCmd)
}

// serviceInstallCmd represents the install service command
var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "install as windows service",
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := viper.GetString("service.name")
		fmt.Println("Install as service:", serviceName)
		exepath, err := exePath()
		if err != nil {
			fmt.Println("could not get the exe path:", err)
			os.Exit(1)
		}

		m, err := mgr.Connect()
		if err != nil {
			fmt.Println("could not connect to mgr:", err)
			os.Exit(1)
		}
		defer m.Disconnect()

		s, err := m.OpenService(serviceName)
		if err == nil {
			s.Close()
			fmt.Printf("service %s already exists", serviceName)
			os.Exit(1)
		}
		config := mgr.Config{
			DisplayName:  serviceName,
			Description:  "myOpenFactory Client to connect to the plattform",
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ServiceRestart,
		}
		if viper.IsSet("service.logon") && viper.IsSet("service.password") {
			config.ServiceStartName = viper.GetString("service.logon")
			config.Password = viper.GetString("service.password")
		}
		s, err = m.CreateService(serviceName, exepath, config, "service", "run", "--config", viper.ConfigFileUsed(), "--name", serviceName)
		if err != nil {
			fmt.Println("could not create service:", err)
			os.Exit(1)
		}
		defer s.Close()

		if err := s.Start(); err != nil {
			fmt.Println("failed to start service:", err)
			os.Exit(1)
		}

		err = eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		if err != nil {
			s.Delete()
			fmt.Println("SetupEventLogSource() failed:", err)
			os.Exit(1)
		}
	},
}

// serviceUninstallCmd represents the uninstall service command
var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "uninstall the windows service",
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := viper.GetString("service.name")
		fmt.Println("Uninstall service:", serviceName)
		m, err := mgr.Connect()
		if err != nil {
			fmt.Println("could not connect to mgr:", err)
			os.Exit(1)
		}
		defer m.Disconnect()

		s, err := m.OpenService(serviceName)
		if err != nil {
			fmt.Println("service not installed:", err)
			os.Exit(1)
		}
		defer s.Close()

		if err = s.Delete(); err != nil {
			fmt.Println("could not delete server:", err)
			os.Exit(1)
		}

		if err = eventlog.Remove(serviceName); err != nil {
			fmt.Println("RemoveEventLogSource() failed:", err)
			os.Exit(1)
		}
	},
}

var serviceRunCmd = &cobra.Command{
	Use:   "run",
	Short: "run the windows service",
	Run: func(cmd *cobra.Command, args []string) {
		logger := cmdpkg.InitializeLogger()
		logger.Infof("Using config: %s", viper.ConfigFileUsed())

		cl, err := cmdpkg.InitializeClient()
		if err != nil {
			logger.Errorf("error while creating client: %v", err)
			os.Exit(1)
		}

		run := svc.Run
		if viper.GetBool("service.debug") {
			run = debug.Run
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("recover client: %v", r)
					logger.Errorf("%s", rdbg.Stack())
				}
			}()
			if err := cl.Run(); err != nil {
				logger.Errorf("error while running client: %v", err)
				os.Exit(1)
			}
		}()

		serviceName := viper.GetString("service.name")
		if err := run(serviceName, &service{client: cl}); err != nil {
			logger.Errorf("service failed: %q: %v", serviceName, err)
			return
		}
		logger.Infof("service stopped: %q", serviceName)
	},
}

func exePath() (string, error) {
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}

	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}

	if filepath.Ext(p) == "" {
		p += ".exe"
		fi, err := os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			err = fmt.Errorf("%s is directory", p)
		}
	}

	return "", err

}

var elog debug.Log

type service struct {
	client *client.Client
}

func (m *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	deadline := 5 * time.Second
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted, WaitHint: uint32(deadline.Seconds()) * 1000}
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				ctx, cancel := context.WithTimeout(context.Background(), deadline)
				defer cancel()
				m.client.Shutdown(ctx)
				return false, 0
			}
		}
	}
}
