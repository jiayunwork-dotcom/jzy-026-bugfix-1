// camkin 是凸轮从动件运动学小服务：只经 HTTP 提供规律档与曲线。
package main

import (
	"log"
	"net/http"
	"os"

	"camkin/internal/httpapi"
	"camkin/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	st, err := store.Open(dataDir)
	if err != nil {
		log.Fatalf("打开数据目录 %s 失败: %v", dataDir, err)
	}
	seeded, err := st.SeedIfEmpty()
	if err != nil {
		log.Fatalf("载入启动配方失败: %v", err)
	}
	if seeded {
		log.Print("配方目录为空，已载入启动配方 default_cycloid（摆线升程，h=10，beta=120 度）")
	}

	srv := httpapi.New(st)
	addr := ":" + port
	log.Printf("camkin 监听 %s，数据目录 %s", addr, dataDir)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatal(err)
	}
}
