// Command openai OpenAI 兼容协议 API（监听 :17200）。
//
// 路由前缀：/v1
// 详见 docs/04-API规范.md §4。
package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kleinai/backend/internal/bootstrap"
	"github.com/kleinai/backend/internal/router"
	"github.com/kleinai/backend/pkg/logger"
)

const serviceName = "openai"

func main() {
	deps, err := bootstrap.Init(serviceName)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	r := router.New(router.Options{ServiceName: serviceName, Deps: deps})
	router.MountOpenAI(r, deps)

	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(deps.Cfg.Server.OpenAIPort),
		Handler:      r,
		ReadTimeout:  deps.Cfg.Server.ReadTimeout,
		WriteTimeout: 600, // 视频生成长任务，下方再覆盖
	}
	srv.WriteTimeout = deps.Cfg.Server.WriteTimeout * 10

	if err := bootstrap.Run(srv, deps.Cfg.Server.ShutdownTimeout); err != nil {
		fmt.Println("server exit error:", err)
	}
}
