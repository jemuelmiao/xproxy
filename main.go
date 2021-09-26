package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"time"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

func getHandler(service *Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		transport := &http.Transport{
			MaxIdleConns:          2000,               // MaxIdleConns 表示空闲KeepAlive长连接数量，为0表示不限制
			MaxIdleConnsPerHost:   100,                // MaxIdleConnsPerHost 表示单Host空闲KeepAlive数量，系统默认为2
			ResponseHeaderTimeout: 60 * time.Second,    // 等待后端响应头部的超时时间
			IdleConnTimeout:       90 * time.Second,   // 长连接空闲超时回收时间
			TLSHandshakeTimeout:   10 * time.Second,   // TLS握手超时时间
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		fmt.Println("handler.....", r.URL.Path)
		var px *Proxy
		for _, p := range service.Proxys {
			matched, e := regexp.MatchString(p.Rule, r.URL.Path)
			if matched && e == nil {
				px = p
				break
			}
		}
		if px == nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("未找到匹配的代理规则："+r.URL.Path))
			return
		}
		proxyReq := r
		if px.Type == "sock5" {
			fmt.Println("go sock proxy:", r.URL.Path)
			last := px.Hops[len(px.Hops)-1]
			dialer, e := proxy.SOCKS5("tcp", last.Listen, nil, proxy.Direct)
			if e != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(e.Error()))
				return
			}
			transport.Dial = dialer.Dial
			proxyReq.URL.Scheme = "http"
			proxyReq.URL.Host = px.Host
		} else {
			fmt.Println("go local proxy:", r.URL.Path)
			proxyReq.URL.Scheme = "http"
			proxyReq.URL.Host = px.Host
		}
		proxyRsp, e := transport.RoundTrip(r)
		if e != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(e.Error()))
			return
		}
		for k := range proxyRsp.Header {
			h := proxyRsp.Header[k]
			for _, v := range h {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(proxyRsp.StatusCode)
		io.Copy(w, proxyRsp.Body)
		proxyRsp.Body.Close()
	}
}
//有上一跳的使用上一跳的Listen，没有上一跳的使用当前跳的Host
func getSshClient(prev, curr *Hop) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		Config:            ssh.Config{},
		User:              curr.User,
		Auth:              []ssh.AuthMethod{ssh.Password(curr.Password)},
		HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
		Timeout:           3*time.Second,
	}
	var host string
	if prev == nil {
		host = curr.Host
	} else {
		host = prev.Listen
	}
	client, e := ssh.Dial("tcp", host, config)
	if e != nil {
		return nil, e
	}
	return client, nil
}
func getSockListen(hop *Hop) func(func(net.Conn)) {
	return func(proc func(net.Conn)) {
		server, err := net.Listen("tcp", hop.Listen)
		if err != nil {
			return
		}
		defer server.Close()
		for {
			client, err := server.Accept()
			if err != nil {
				return
			}
			go proc(client)
		}
	}
}
func getSockForward(prev, curr, next *Hop) func(net.Conn) {
	return func(conn net.Conn) {
		defer func() {
			if conn != nil {
				_ = conn.Close()
			}
		}()
		//这里使用当前Host
		client, err := getSshClient(prev, curr)
		if err != nil {
			return
		}
		defer client.Close()
		server, err := client.Dial("tcp", next.Host)
		if err != nil {
			return
		}
		defer server.Close()
		go func() {
			_, _ = io.Copy(server, conn)
		}()
		_, _ = io.Copy(conn, server)
	}
}
func getSockProxy(prev, curr *Hop) func(net.Conn) {
	return func(conn net.Conn) {
		defer func() {
			if conn != nil {
				_ = conn.Close()
			}
		}()
		var b [1024]byte
		n, err := conn.Read(b[:])
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte{0x05, 0x00})
		n, err = conn.Read(b[:])
		if err != nil{
			return
		}
		var addr string
		switch b[3] {
		case 0x01:
			type sockIP struct {
				A, B, C, D byte
				PORT       uint16
			}
			sip := sockIP{}
			if err := binary.Read(bytes.NewReader(b[4:n]), binary.BigEndian, &sip); err != nil {
				return
			}
			addr = fmt.Sprintf("%d.%d.%d.%d:%d", sip.A, sip.B, sip.C, sip.D, sip.PORT)
		case 0x03:
			host := string(b[5 : n-2])
			var port uint16
			err = binary.Read(bytes.NewReader(b[n-2:n]), binary.BigEndian, &port)
			if err != nil {
				return
			}
			addr = fmt.Sprintf("%s:%d", host, port)
		}
		//这里使用上一跳Listen
		client, err := getSshClient(prev, curr)
		if err != nil {
			return
		}
		defer client.Close()
		server, err := client.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer server.Close()
		_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		go func() {
			_, _ = io.Copy(server, conn)
		}()
		_, _ = io.Copy(conn, server)
	}
}

func main() {
	for _, env := range envs {
		if env.Disable {
			continue
		}
		for _, service := range env.Services {
			if service.Disable {
				continue
			}
			for _, p := range service.Proxys {
				if p.Type != "sock5" || len(p.Hops) == 0 {
					continue
				}
				for i:=0; i<len(p.Hops); i++ {
					var prev, curr, next *Hop
					if i > 0 {
						prev = p.Hops[i-1]
					}
					curr = p.Hops[i]
					if i < len(p.Hops)-1 {
						next = p.Hops[i+1]
					}
					if next == nil { //当前为最后一跳
						go func(pre, cur *Hop) {
							for {
								getSockListen(cur)(getSockProxy(pre, cur))
								time.Sleep(time.Second)
							}
						}(prev, curr)
					} else {
						go func(pre, cur, nex *Hop) {
							for {
								getSockListen(curr)(getSockForward(pre, cur, nex))
								time.Sleep(time.Second)
							}
						}(prev, curr, next)
					}
				}
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/", getHandler(service))
			go http.ListenAndServe(service.Listen, mux)
		}
	}
	select {}
}