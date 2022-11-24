package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/arakos/crd-to-openapischema/pkg/generator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "crd-to-openapischema",
		Short:         "",
		Long:          `.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlags(cmd.Flags())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.GetViper()

			_, err := generator.Generate(args[0], v.GetString("output-dir"))
			return err
		},
	}

	cobra.OnInitialize(initConfig)

	cmd.Flags().String("output-dir", "./", "directory to save the schemas in")

	viper.BindPFlags(cmd.Flags())

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	return cmd
}

func InitAndExecute() {
	if err := RootCmd().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func initConfig() {
	viper.SetEnvPrefix("CRDTOOPENAPISCHEMA")
	viper.AutomaticEnv()
}
