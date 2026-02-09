package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	// 1. 配置支持 HTTP/2 的自定义 Transport
	// 必须强制开启 TLS，因为 H2 几乎只在 HTTPS 上运行
	t := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	// 确保启用 HTTP/2
	// http2.ConfigureTransport(t) // 默认标准库已经支持

	hc := &http.Client{
		Transport: t,
	}

	// 你的 Cloudflare Worker 地址
	baseURL := "https://polished-scene-73d0.edtxtg.workers.dev"

	var wg sync.WaitGroup
	wg.Add(2)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// --- 任务 1: 连接 /ws1 并发送 start ---
	go func() {
		defer wg.Done()
		opts := &websocket.DialOptions{
			HTTPClient: hc, // 关键：使用同一个 HTTP Client 以复用 H2 连接
		}
		
		c, _, err := websocket.Dial(ctx, baseURL+"/ws1", opts)
		if err != nil {
			fmt.Printf("❌ WS1 连接失败: %v\n", err)
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// 发送 start 信号触发 TCP 连接
		err = c.Write(ctx, websocket.MessageText, []byte("start"))
		if err != nil {
			fmt.Printf("❌ WS1 发送失败: %v\n", err)
			return
		}

		// 读取返回的 instanceId
		var v interface{}
		err = wsjson.Read(ctx, c, &v)
		if err == nil {
			fmt.Printf("✅ WS1 响应: %v\n", v)
		}
	}()

	// 稍微延迟一点点，确保 WS1 的 TCP 连接先跑起来
	time.Sleep(time.Second * 1)

	// --- 任务 2: 连接 /ws2 并发送 check ---
	go func() {
		defer wg.Done()
		opts := &websocket.DialOptions{
			HTTPClient: hc, // 关键：复用同一个底层的 TCP/H2 连接
		}

		c, _, err := websocket.Dial(ctx, baseURL+"/ws2", opts)
		if err != nil {
			fmt.Printf("❌ WS2 连接失败: %v\n", err)
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// 发送 check 信号
		err = c.Write(ctx, websocket.MessageText, []byte("check"))
		if err != nil {
			fmt.Printf("❌ WS2 发送失败: %v\n", err)
			return
		}

		// 读取返回的 instanceId 和 TCP 状态
		var v interface{}
		err = wsjson.Read(ctx, c, &v)
		if err == nil {
			fmt.Printf("✅ WS2 响应: %v\n", v)
		}
	}()

	wg.Wait()
}
