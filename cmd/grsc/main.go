package main

import (
	"context"
	"net"
	"time"

	"github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"github.com/howmp/reality"
	"github.com/howmp/reality/cmd"
	"github.com/sirupsen/logrus"
)

func main() {
	config, err := reality.UnmarshalClientConfig(cmd.ConfigDataPlaceholder)
	if err != nil {
		println(err.Error())
		return
	}
	logger := reality.GetLogger(config.Debug)
	logger.Infof("server addr: %s, sni: %s", config.ServerAddr, config.SNI)

	socksServer, err := socks5.New(&socks5.Config{})
	if err != nil {
		logger.Fatalln(err)
	}
	c := client{logger: logger, config: config, socksServer: socksServer}

	// 重连间隔表（秒）
	retryTable := []int{5, 10, 20, 40, 80, 160, 320, 640}
	retryIndex := 0

	for {
		success := false
		err = c.serve(&success)
		if err != nil {
			c.logger.Errorf("serve: %v", err)

			// 如果刚刚成功连接过一次，重置 retryIndex
			if success {
				retryIndex = 0
			}

			wait := retryTable[retryIndex]
			c.logger.Infof("sleep %ds", wait)
			time.Sleep(time.Duration(wait) * time.Second)

			// 递增重试次数（最多到最后一个 640 秒）
			if retryIndex < len(retryTable)-1 {
				retryIndex++
			}
		} else {
			// serve() 正常退出（极少见），重置重连
			retryIndex = 0
		}
	}
}

type client struct {
	logger      logrus.FieldLogger
	config      *reality.ClientConfig
	socksServer *socks5.Server
}

// serve 修改为传入 success 指针，用于标记是否成功连接
func (c *client) serve(success *bool) error {
	c.logger.Infoln("try connect to server")
	client, err := reality.NewClient(context.Background(), c.config)
	if err != nil {
		return err
	}
	*success = true // 成功建立连接
	c.logger.Infoln("server connected")
	defer client.Close()

	session, err := yamux.Client(client, nil)
	if err != nil {
		return err
	}
	defer session.Close()

	for {
		stream, err := session.Accept()
		if err != nil {
			return err
		}
		c.logger.Infof("new client %s", stream.RemoteAddr())
		go c.handleStream(stream)
	}
}

func (c *client) handleStream(conn net.Conn) {
	defer conn.Close()
	c.socksServer.ServeConn(conn)
}
