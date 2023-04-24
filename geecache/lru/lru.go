package lru

import (
	"container/list"
	"fmt"
)

type Cache struct {
	maxBytes  int64
	nbytes    int64
	ll        *list.List
	cache     map[string]*list.Element
	OnEvicted func(key string, value Value)
}

type entry struct {
	key   string
	value Value
}

type Value interface {
	Len() int
}

func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	// 实现Cache的构造函数，这是Go语言中类的构造方法。注意,Go语言中没有函数重载，因此不同的构造方法需要使用不同的函数名
	fmt.Printf("创建一个对象,maxBytes=%v\n", maxBytes)
	return &Cache{
		maxBytes:  maxBytes, // maxBytes=0表示不限制大小
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache) Get(key string) (value Value, ok bool) {
	// Go语言中类方法的实现之一，可以支持读写
	// 如果要只读，则不传指针，即(c Cache)
	if ele, ok := c.cache[key]; ok {
		// 先从c的字典里取出来，看ok是不是nil，不是nil说明存在
		c.ll.MoveToBack(ele)
		// 将该列表中的该值移到列表尾部
		kv := ele.Value.(*entry)
		// list返回一个struct结构体，名为Element，具有一个接口，名为Value
		// 使用.(*entry)进行强制类型转换为entry
		return kv.value, true
	}
	return
}

func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		fmt.Printf("键值对%v:%v已删除\n", kv.key, kv.value)
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

func (c *Cache) Add(key string, value Value) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		tmp := kv.value
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
		fmt.Printf("%v已存在,原值为%v,现在修改为%v\n", key, tmp, value)
	} else {
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + int64(value.Len())
		fmt.Printf("已添加%v:%v\n", key, value)
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()
}
