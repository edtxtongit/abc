package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	targetURL := flag.String("url", "", "Cloudflare Worker URL")
	flag.Parse()

	if *targetURL == "" {
		fmt.Println("❌ 请提供 -url 参数")
		return
	}

	url := strings.TrimRight(*targetURL, "/")
	hc := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			ForceAttemptHTTP2: true, // 强制尝试 H2
		},
	}

	var wg sync.WaitGroup
	// 用于通知 WS1：WS2 已经检查完毕，你可以关了
	doneSignal := make(chan struct{})

	wg.Add(2)

	// --- WS1: 建立连接并维持 ---
	go func() {
		defer wg.Done()
		ctx := context.Background()
		c, _, err := websocket.Dial(ctx, url+"/ws1", &websocket.DialOptions{HTTPClient: hc})
		if err != nil {
			fmt.Printf("❌ WS1 连接失败: %v\n", err)
			return
		}
		// 确保最后关闭
		defer c.Close(websocket.StatusNormalClosure, "done")

		fmt.Println("📡 WS1 已连接，发送 start...")
		c.Write(ctx, websocket.MessageText, []byte("start"))

		var res interface{}
		wsjson.Read(ctx, c, &res)
		fmt.Printf("✅ WS1 初始响应: %v\n", res)

		fmt.Println("⏳ WS1 正在保持连接，等待 WS2 检查...")
		
		// 阻塞在这里，直到收到 WS2 完成的信号
		select {
		case <-doneSignal:
			fmt.Println("👋 WS1 收到完成信号，准备退出")
		case <-time.After(15 * time.Second):
			fmt.Println("⏰ WS1 等待超时")
		}
	}()

	// 延迟 2 秒，确保 WS1 稳定
	time.Sleep(2 * time.Second)

	// --- WS2: 建立连接进行 check ---
	go func() {
		defer wg.Done()
		defer close(doneSignal) // 执行完后通知 WS1

		ctx := context.Background()
		c, _, err := websocket.Dial(ctx, url+"/ws2", &websocket.DialOptions{HTTPClient: hc})
		if err != nil {
			fmt.Printf("❌ WS2 连接失败: %v\n", err)
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		fmt.Println("📡 WS2 已连接，发送 check...")
		c.Write(ctx, websocket.MessageText, []byte("check"))

		var res interface{}
		if err := wsjson.Read(ctx, c, &res); err == nil {
			fmt.Printf("🎯 WS2 检查结果: %v\n", res)
		}
	}()

	wg.Wait()
}
