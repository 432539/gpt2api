// Command api 用户端 API 服务（监听 :17180）。
//
// 路由前缀：/api/v1
// 详见 docs/04-API规范.md §2。
package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kleinai/backend/internal/bootstrap"
	"github.com/kleinai/backend/internal/router"
	"github.com/kleinai/backend/pkg/logger"
)

const serviceName = "api"

func main() {
	deps, err := bootstrap.Init(serviceName)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	r := router.New(router.Options{ServiceName: serviceName, Deps: deps})
	router.MountAPI(r, deps)

	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(deps.Cfg.Server.APIPort),
		Handler:      r,
		ReadTimeout:  deps.Cfg.Server.ReadTimeout,
		WriteTimeout: deps.Cfg.Server.WriteTimeout,
	}

	if err := bootstrap.Run(srv, deps.Cfg.Server.ShutdownTimeout); err != nil {
		fmt.Println("server exit error:", err)
	}
}
