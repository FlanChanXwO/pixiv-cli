package environment

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	defaultValue := regexp.MustCompile(`(?m)\(default (.*)\)$`)
	cobra.AddTemplateFunc("localizedFlagUsages", func(flags *pflag.FlagSet, locale string) string {
		return defaultValue.ReplaceAllStringFunc(flags.FlagUsages(), func(value string) string {
			return fmt.Sprintf(i18n.Locale(locale).Text("help.default_value"), strings.TrimSuffix(strings.TrimPrefix(value, "(default "), ")"))
		})
	})
}

// LocalizeCommands 在命令树组装完毕后解析帮助消息键，Cobra 的内建项也使用同一语言。
func LocalizeCommands(cmd *cobra.Command, locale i18n.Locale) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations["interfaceLanguage"] = locale.String()
	for _, field := range []*string{&cmd.Short, &cmd.Long} {
		if strings.HasPrefix(*field, "help.") {
			*field = locale.Text(*field)
		}
	}
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()
	localize := func(flag *pflag.Flag) {
		if strings.HasPrefix(flag.Usage, "flag.") {
			flag.Usage = locale.Text(flag.Usage)
		}
		if flag.Name == "help" {
			flag.Usage = fmt.Sprintf(locale.Text("help.for_command"), cmd.Name())
		}
		if flag.Name == "version" {
			flag.Usage = fmt.Sprintf(locale.Text("help.version"), cmd.Name())
		}
	}
	cmd.Flags().VisitAll(localize)
	cmd.PersistentFlags().VisitAll(localize)
	template := cmd.UsageTemplate()
	for _, pair := range []struct{ from, key string }{
		{"Usage:", "help.usage"}, {"Aliases:", "help.aliases"}, {"Examples:", "help.examples"},
		{"Available Commands:", "help.commands"}, {"Additional Commands:", "help.additional_commands"},
		{"Global Flags:", "help.global_flags"}, {"Flags:", "help.flags"},
		{"Additional help topics:", "help.topics"},
		{`Use "{{.CommandPath}} [command] --help" for more information about a command.`, "help.more_info"},
	} {
		template = strings.ReplaceAll(template, pair.from, locale.Text(pair.key))
	}
	template = strings.ReplaceAll(template, ".LocalFlags.FlagUsages", "localizedFlagUsages .LocalFlags .Annotations.interfaceLanguage")
	template = strings.ReplaceAll(template, ".InheritedFlags.FlagUsages", "localizedFlagUsages .InheritedFlags .Annotations.interfaceLanguage")
	cmd.SetUsageTemplate(template)
	for _, child := range cmd.Commands() {
		LocalizeCommands(child, locale)
	}
}
