package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/weibaohui/sc/config"
	"github.com/weibaohui/sc/counter"
	"github.com/weibaohui/sc/file"
	"github.com/weibaohui/sc/git"
)

var ignoreHide = true
var debug = false
var path string

var rootCmd = &cobra.Command{
	Use:   "sc",
	Short: "统计源码行数",
	Long:  "按文件夹统计源码行数",
	Run: func(cmd *cobra.Command, args []string) {
		// git.GetInstance()
		fmt.Println(git.GetInstance().Execute().String())
		cfg := config.GetInstance()
		cfg.IgnoreHide = ignoreHide
		cfg.Debug = debug
		// todo 初始目录 做到config 中，两种统计 从config中取
		initFolder := &file.Folder{
			FullPath: path,
			Hidden:   false,
		}
		initFolder.Execute()
		fmt.Println(counter.GetInstance().Sum())
	},
}

// Execute 执行
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "调试")
	rootCmd.Flags().StringVarP(&path, "path", "p", ".", "扫描路径")
}
