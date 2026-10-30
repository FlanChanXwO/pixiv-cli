package detail

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/spf13/cobra"
)

// markNDJSON 记录 detail 的输出模式，只在确实要写记录流时安装 broken-pipe 策略。
func (a commandRunner) markNDJSON(cmd *cobra.Command, enabled bool) {
	environment.SetOutputNDJSON(cmd, enabled)
	if enabled && a.env.EnablePipelineSignal != nil {
		a.env.EnablePipelineSignal()
	}
}
