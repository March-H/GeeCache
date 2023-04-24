package singleflight

import "sync"

// 代表正在进行中，或已经结束的请求，使用sync.WaitGroup锁避免重入
type call struct {
	wg sync.WaitGroup
	// 等待组的Wait函数在等待组计数器不等于0时阻塞直到变成0
	val interface{}
	err error
}

// 管理不同key的请求
type Group struct {
	mu sync.Mutex // protects m
	m  map[string]*call
}

//针对相同的key，无论该函数被调用多少次，函数fn都只会调用一次，等待fn调用结束了，返回值或错误
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	// 当执行该函数时，先判断是否已经有对应的请求正在进行中，加锁阻塞
	g.mu.Lock()
	if g.m == nil {
		// 延迟初始化的方法
		// 如果 g.m 为空，表示没有请求正在进行，那么创建一个map
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		// 如果 ok ， 说明有请求正在进行，取消锁，让其它同样的请求都能到达这一步
		g.mu.Unlock()
		c.wg.Wait()         // 如果请求正在进行中，则等待
		return c.val, c.err // 请求结束，返回结果
	}
	// 不 ok ，则继续阻塞 g.mu
	// 新建请求并加锁
	c := new(call)
	c.wg.Add(1)  // 发起请求前加锁
	g.m[key] = c // 添加到 g.m，表明 key 已经有对应的请求在处理
	// 由于该请求已经加进 g.map 里了，所以 g.mu 可以解锁了，方便其它同样的请求进行
	// 解锁后，该请求由于已经有记录，其它同样的请求会阻塞在 30行 处
	g.mu.Unlock()

	c.val, c.err = fn() // 调用 fn，发起请求
	c.wg.Done()         // 请求结束

	g.mu.Lock()
	delete(g.m, key) // 更新 g.m
	// 这里只做高并发时检查的功能，不进行存储。所有数据都以Cache中为主
	g.mu.Unlock()

	return c.val, c.err // 返回结果
}
