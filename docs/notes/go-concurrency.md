# Go 并发编程要点

## goroutine 与 channel

goroutine 是 Go 的轻量级线程，`go f(x)` 一行即可启动。channel 是 goroutine
间的通信管道："不要通过共享内存来通信，而要通过通信来共享内存"。

- 无缓冲 channel：发送阻塞直到有人接收（同步点）
- 带缓冲 channel：`make(chan int, 3)` 缓冲满前发送不阻塞
- 关闭：发送方 `close(ch)`；接收方 `v, ok := <-ch` 中 ok 为 false 表示已关闭

## select 多路复用

`select` 同时等待多个 channel，哪个先就绪就执行哪个分支；带 `default`
分支则都不就绪时立即返回，常用于非阻塞收发与超时控制：

    select {
    case v := <-ch1:
    case <-time.After(time.Second):
    }

## 常见坑

1. 主 goroutine 退出会带走所有子 goroutine——需要 `sync.WaitGroup` 等待
2. 向已关闭的 channel 发送会 panic——关闭职责只属于唯一的发送方
3. 在循环里启动 goroutine 捕获循环变量：Go 1.22 起循环变量每轮独立，
   老版本需要 `v := v` 快照
4. goroutine 泄漏：阻塞在无人接收的 channel 上——用 context 取消或超时兜底

## 经验法则

能用几行代码达到的就别上协程级复杂度；并发原语首选 channel，
共享状态保护次选 `sync.Mutex`。
