package pool

import (
	"errors"
	"sync"
	"time"
)

// Config 连接池配置
type Config struct {
	//最小连接数
	InitCap int
	//最大连接数
	MaxCap int
	//生成连接的方法
	Factory func() (interface{}, error)
	//关闭连接的方法
	Close func(interface{}) error
	//探活方法
	Alive func() error
	//最大空闲时间
	IdleTimeout time.Duration
}
type channelPool struct {
	//锁
	mu sync.Mutex
	//连接池
	conns chan *idleConn
	//生成连接的方法
	factory func() (interface{}, error)
	//关闭连接的方法
	close func(interface{}) error
	//探活方法
	alive func() error
	//最大空闲时间
	idleTimeout time.Duration
}
type idleConn struct {
	//真正的连接
	conn interface{}
	//创建时间
	t time.Time
}

func NewChannelPool(poolConfig *Config) (Pool, error) {
	if poolConfig.InitCap <= 0 || poolConfig.MaxCap <= 0 || poolConfig.InitCap > poolConfig.MaxCap {
		return nil, errors.New("poolConfig.InitCap or poolConfig.MaxCap is error")
	}
	cPool := &channelPool{
		conns:       make(chan *idleConn, poolConfig.MaxCap),
		factory:     poolConfig.Factory,
		close:       poolConfig.Close,
		idleTimeout: poolConfig.IdleTimeout,
	}
	for i := 0; i < poolConfig.InitCap; i++ {
		conn, err := cPool.factory()
		if err != nil {
			cPool.Release()
			return nil, err
		}
		cPool.conns <- &idleConn{conn: conn, t: time.Now()}
	}
	return cPool, nil
}

func (cPool *channelPool) getConns() chan *idleConn {
	cPool.mu.Lock()
	conns := cPool.conns
	cPool.mu.Unlock()
	return conns
}
func (cPool *channelPool) Get() (interface{}, error) {
	conns := cPool.getConns()
	if conns == nil {
		return nil, ErrClose
	}
	for {
		select {
		case wrapConn := <-conns:
			if wrapConn == nil {
				return nil, ErrClose
			}
			//判断是否超时
			if timeOut := cPool.idleTimeout; timeOut > 0 {
				if wrapConn.t.Add(timeOut).Before(time.Now()) {
					cPool.close(wrapConn)
					continue
				}
			}
			return wrapConn.conn, nil
		default:
			conn, err := cPool.factory()
			if err != nil {
				return nil, err
			}
			return conn, nil
		}

	}

}

func (cPool *channelPool) Put(conn interface{}) error {
	if conn == nil {
		err := errors.New("conn is now")
		return err
	}
	cPool.mu.Lock()
	defer cPool.mu.Unlock()

	if cPool.conns == nil {

		return cPool.Close(conn)
	}

	select {
	case cPool.conns <- &idleConn{conn: conn, t: time.Now()}:
		return nil
	default:
		return cPool.Close(conn)
	}
}
func (cPool *channelPool) Close(conn interface{}) error {
	if conn == nil {
		err := errors.New("conn is nil")
		return err
	}
	return cPool.close(conn)
}
func (cPool *channelPool) Release() {
	cPool.mu.Lock()
	conns := cPool.conns
	closeFunc := cPool.close
	cPool.conns = nil
	cPool.close = nil
	cPool.factory = nil
	cPool.mu.Unlock()

	close(conns)
	for wrapConn := range conns {
		closeFunc(wrapConn)
	}
}
func (cPool *channelPool) Len() int {
	return len(cPool.getConns())
}
