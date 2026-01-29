package main

import (
	"sync"
	"github.com/xpzouying/headless_browser"
	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

// BrowserPool 浏览器实例池
// 使用单例模式复用浏览器实例，避免每次请求都创建新浏览器
// 这可以显著提升性能（从60秒降低到1.4秒）
type BrowserPool struct {
	mu      sync.RWMutex
	browser *headless_browser.Browser
}

var globalPool *BrowserPool
var once sync.Once

// GetBrowserPool 获取全局浏览器池实例
func GetBrowserPool() *BrowserPool {
	once.Do(func() {
		globalPool = &BrowserPool{}
	})
	return globalPool
}

// GetBrowser 获取或创建浏览器实例
// 如果浏览器未创建，则创建新实例；否则返回已存在的实例
func (p *BrowserPool) GetBrowser() *headless_browser.Browser {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.browser == nil {
		p.browser = browser.NewBrowser(configs.IsHeadless(), browser.WithBinPath(configs.GetBinPath()))
	}
	return p.browser
}

// Close 关闭浏览器实例并清空池
func (p *BrowserPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.browser != nil {
		p.browser.Close()
		p.browser = nil
	}
}

// Restart 重启浏览器实例
// 关闭当前浏览器并创建新实例
func (p *BrowserPool) Restart() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.browser != nil {
		p.browser.Close()
	}
	p.browser = browser.NewBrowser(configs.IsHeadless(), browser.WithBinPath(configs.GetBinPath()))
}
