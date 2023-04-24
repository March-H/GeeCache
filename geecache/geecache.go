package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	"geecache/singleflight"
	"log"
	"sync"
)

type Group struct {
	name      string
	getter    Getter     // 缓存未命中时获取源数据的回调
	mainCache cache      //本地Cache
	peers     PeerPicker //其它分布式节点
	// use singleflight.Group to make sure that
	// each key is only fetched once
	loader *singleflight.Group
}

// 定义了一个接口，该接口具有一个Get函数，实现了该Get函数的，都可以是Getter类型
type Getter interface {
	Get(key string) ([]byte, error)
}

// 定义了一个函数类型
type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	// 对groups进行写操作，需要获得写锁
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
		loader:    &singleflight.Group{},
	}
	groups[name] = g
	return g
}

// GetGroup returns the named group which was previously created with NewGroup, or
// nil if there's no such group.
func GetGroup(name string) *Group {
	// 需要进行读操作，获得读锁
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

// Get func 从mainCache获取缓存值，如果没有，则尝试从其它分布式节点加载缓存值；其它分布式节点没有缓存值，则从本地数据库加载并缓存
func (g *Group) Get(key string) (ByteView, error) { // 获取缓存信息
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
		// fmt.Errorf()函数将字符串作为错误返回
	}

	if v, ok := g.mainCache.get(key); ok {
		// 从mainCache中读取该键值对，如果有，直接返回
		log.Println("[GeeCache] hit")
		return v, nil
	}

	return g.load(key)
	// 如果没有，则尝试从其它分布式节点加载
}

// 从其它分布式节点加载缓存，如果有，直接返回该缓存值，如果没有，调用回调函数从本地数据库加载，并添加进主缓存中
func (g *Group) load(key string) (value ByteView, err error) {
	// each key is only fetched once (either locally or remotely)
	// regardless of the number of concurrent callers.
	// 尽管有多个并发请求会到达这里，但是操作的是同一块内存，请求的是同一个锁
	// 所以只会有第一个请求能够请求数据，其它的会阻塞
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok {
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}

		return g.getLocally(key)
	})
	if err == nil {
		return viewi.(ByteView), nil
	}

	return
}

// 从本地数据库加载值，并添加进缓存中
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
		// 报错返回
	}
	c := cloneBytes(bytes)
	// fmt.Printf("%p %p\n", &c, &bytes)
	value := ByteView{b: c}
	// 这里强制类型转换赋值似乎会复制一份再赋值，而不是直接将切片映射过去，输出可以发现b和c的地址是不一样的
	// fmt.Printf(("%p\n"), &value.b)
	g.populateCache(key, value)
	return value, nil
}

// 将数据添加进缓存中
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}

// 注册其它分布式节点，只允许登记一次
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

// 从分布式节点获取数据
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	req := &pb.Request{
		Group: g.name,
		Key:   key,
	}
	res := &pb.Response{}
	err := peer.Get(req, res)
	// 调用http.Get方法从分布式节点获得缓存
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: res.Value}, nil
}
