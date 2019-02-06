package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/zetaab/kops-autoscaler-openstack/pkg/autoscaler"
)

// Execute will execute basically the whole application
func Execute() {
	options := &autoscaler.Options{}
	flag.Lookup("logtostderr").Value.Set("true")
	glog.Infof("Starting application...\n")
	glog.Flush()
	rootCmd := &cobra.Command{
		Use:   "kops-autoscaling-openstack",
		Short: "Provide autoscaling capability to kops openstack",
		Long:  `Provide autoscaling capability to kops openstack`,
		Run: func(cmd *cobra.Command, args []string) {
			err := validate(options)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n%v\n", err)
				os.Exit(1)
				return
			}

			err = autoscaler.Run(options)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n%v\n", err)
				os.Exit(1)
				return
			}
		},
	}

	rootCmd.Flags().IntVar(&options.Sleep, "sleep", 45, "Sleep between executions")
	rootCmd.Flags().StringVar(&options.StateStore, "state-store", os.Getenv("KOPS_STATE_STORE"), "KOPS State store")
	rootCmd.Flags().StringVar(&options.AccessKey, "access-key", os.Getenv("S3_ACCESS_KEY_ID"), "S3 access key")
	rootCmd.Flags().StringVar(&options.SecretKey, "secret-key", os.Getenv("S3_SECRET_ACCESS_KEY"), "S3 secret key")
	rootCmd.Flags().StringVar(&options.CustomEndpoint, "custom-endpoint", os.Getenv("S3_ENDPOINT"), "S3 custom endpoint")
	rootCmd.Flags().StringVar(&options.ClusterName, "name", os.Getenv("NAME"), "Name of the kubernetes kops cluster")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func validate(options *autoscaler.Options) error {
	if options.ClusterName == "" {
		return fmt.Errorf("Please set NAME to env variable or as start flag")
	}
	if options.StateStore == "" {
		return fmt.Errorf("Please set KOPS_STATE_STORE to env variable or as start flag")
	}
	// set env variable, needed by kops libraries
	if os.Getenv("KOPS_STATE_STORE") == "" && options.StateStore != "" {
		err := os.Setenv("KOPS_STATE_STORE", options.StateStore)
		if err != nil {
			return err
		}
	}

	if strings.HasPrefix(options.StateStore, "s3://") || strings.HasPrefix(options.StateStore, "do://") {
		if options.AccessKey == "" {
			return fmt.Errorf("Please set S3_ACCESS_KEY_ID to env variable or as start flag")
		}

		if os.Getenv("S3_ACCESS_KEY_ID") == "" && options.AccessKey != "" {
			err := os.Setenv("S3_ACCESS_KEY_ID", options.AccessKey)
			if err != nil {
				return err
			}
		}

		if options.SecretKey == "" {
			return fmt.Errorf("Please set S3_SECRET_ACCESS_KEY to env variable or as start flag")
		}

		if os.Getenv("S3_SECRET_ACCESS_KEY") == "" && options.SecretKey != "" {
			err := os.Setenv("S3_SECRET_ACCESS_KEY", options.SecretKey)
			if err != nil {
				return err
			}
		}
	}

	if os.Getenv("KOPS_FEATURE_FLAGS") == "" {
		err := os.Setenv("KOPS_FEATURE_FLAGS", "AlphaAllowOpenstack,+EnableExternalCloudController")
		if err != nil {
			return err
		}
	}

	// TODO: validate openstack env variables
	return nil
}
