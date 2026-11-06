// scherun 是一个后台任务管理器: 托盘常驻, 界面在浏览器里.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/azazo1/scherun/internal/app"
	"github.com/azazo1/scherun/internal/appdirs"
	"github.com/azazo1/scherun/internal/version"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

// newRootCommand 构建命令行.
func newRootCommand() *cobra.Command {
	var (
		bind      string
		noBrowser bool
		logLevel  string
		showVer   bool
	)

	root := &cobra.Command{
		Use:   "scherun",
		Short: "后台任务管理器, 托盘常驻, 界面在浏览器中",
		Long: "scherun 在系统托盘常驻, 按触发方式管理后台任务.\n" +
			"管理界面运行在本地浏览器中, 地址与访问令牌见托盘菜单与日志.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVer {
				v, commit, buildTime := version.Info()
				fmt.Printf("scherun %s (commit %s, built %s)\n", v, commit, buildTime)
				return nil
			}
			return app.Run(app.Options{
				Bind:      bind,
				NoBrowser: noBrowser,
				LogLevel:  logLevel,
			})
		},
	}

	root.Flags().StringVar(&bind, "bind", "", "覆盖本地监听地址, 例如 127.0.0.1:20008")
	root.Flags().BoolVar(&noBrowser, "no-browser", false, "启动时不自动打开浏览器")
	root.Flags().StringVar(&logLevel, "log-level", "", "日志级别: debug / info / warn / error")
	root.Flags().BoolVarP(&showVer, "version", "V", false, "打印版本后退出")

	root.AddCommand(newPathsCommand())
	return root
}

// newPathsCommand 打印各类目录与文件路径, 便于排查问题.
func newPathsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "打印配置, 数据, 日志与运行状态路径",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printers := []struct {
				name string
				fn   func() (string, error)
			}{
				{"配置目录", appdirs.ConfigDir},
				{"配置文件", appdirs.ConfigPath},
				{"数据目录", appdirs.DataDir},
				{"数据库", appdirs.DatabasePath},
				{"日志文件", appdirs.LogFile},
				{"运行状态", appdirs.RuntimePath},
				{"单实例锁", appdirs.LockPath},
			}
			for _, item := range printers {
				path, err := item.fn()
				if err != nil {
					fmt.Printf("%-10s <错误: %v>\n", item.name, err)
					continue
				}
				fmt.Printf("%-10s %s\n", item.name, path)
			}
			return nil
		},
	}
}
