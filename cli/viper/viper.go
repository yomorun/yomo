package viper

import (
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	InitViper  = viper.New()
	RunViper   = viper.New()
	DevViper   = viper.New()
	BuildViper = viper.New()
)

func BindPFlags(v *viper.Viper, flags *pflag.FlagSet) {
	// set default values
	flags.VisitAll(func(f *pflag.Flag) {
		if f.DefValue != "" {
			v.SetDefault(f.Name, f.DefValue)
		}
	})
	// bind environment variables
	// v.AllowEmptyEnv(true)
	v.SetEnvPrefix("YOMO_SFN")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.BindPFlags(flags)
	v.AutomaticEnv()
}
