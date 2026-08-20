// Command publicapi 打印 v1 公开 SDK 包的导出符号清单，作为 public API
// inventory golden 的生成与校验工具。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/publicapi"
)

func main() {
	dir := flag.String("dir", ".", "repository root")
	check := flag.Bool("check", false, "verify the generated inventory matches the golden file")
	golden := flag.String("golden", "", "golden file path to write or compare")
	flag.Parse()

	inventory := publicapi.Inventory(*dir)
	content := publicapi.Render(inventory)

	if *golden != "" {
		if *check {
			existing, err := os.ReadFile(*golden)
			if err != nil {
				fmt.Fprintln(os.Stderr, "read golden:", err)
				os.Exit(1)
			}
			if string(existing) != content {
				fmt.Fprintln(os.Stderr, "public API inventory drifted from golden")
				os.Exit(1)
			}
			fmt.Println("public API inventory matches golden")
			return
		}
		if err := os.WriteFile(*golden, []byte(content), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write golden:", err)
			os.Exit(1)
		}
		fmt.Println("wrote golden inventory")
		return
	}
	fmt.Print(content)
}
