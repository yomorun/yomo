package viper

import (
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	RunViper   = viper.New()
	DevViper   = viper.New()
	BuildViper = viper.New()
)

func BindPFlags(v *viper.Viper, flags *pflag.FlagSet) {
	// bind environment variables
	// v.AllowEmptyEnv(true)
	v.SetEnvPrefix("YOMO_SFN")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.BindPFlags(flags)
	v.AutomaticEnv()
}
