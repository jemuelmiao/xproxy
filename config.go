package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
)

//环境
type Env struct {
	Name 		string		`json:"name"`
	Disable		bool		`json:"disable"`
	Services	[]*Service	`json:"services"`
	Hops 		[]*Hop		`json:"hops"`
}

//服务
type Service struct {
	Name 	string		`json:"name"`
	Disable	bool		`json:"disable"`
	Listen 	string		`json:"listen"` //本地浏览器访问地址
	Proxys	[]*Proxy	`json:"proxys"` //根据不同规则的代理列表
}

//代理信息
type Proxy struct {
	Type 		string		`json:"type"` //代理类型：http、sock5
	Rule		string		`json:"rule"` //代理规则
	Host 		string		`json:"host"` //服务所在局域网地址，例如本地：127.0.0.1:8080，例如云环境：172.31.16.7:8090
	Hops 		[]*Hop		`json:"hops"` //跳跃信息，仅sock5有效
	UseEnvHops	bool		`json:"use_env_hops"` //是否使用环境hops，仅sock5有效
}

//ssh信息
type Hop struct {
	Host 		string	`json:"host"`
	User 		string	`json:"user"`
	Password	string	`json:"password"`
	Listen 		string	`json:"listen"` //本地sock代理地址，如：127.0.0.1:1080
}

var envs []*Env

func init() {
	type Config struct {
		Envs	[]*Env
	}
	var cfg Config
	cfgFile := "./config.toml"
	if _, e := toml.DecodeFile(cfgFile, &cfg); e != nil {
		fmt.Println("读取配置文件失败：", e)
		os.Exit(1)
	}
	envs = cfg.Envs
	deduplicate := make(map[string]bool)
	for _, env := range envs {
		if env.Disable {
			continue
		}
		for _, service := range env.Services {
			if service.Disable {
				continue
			}
			for _, px := range service.Proxys {
				if px.Type != "http" && px.Type != "https" && px.Type != "sock5" {
					fmt.Println("代理类型必须为http、https、sock5：", px.Type)
					os.Exit(1)
				}
				if px.Type == "sock5" && px.UseEnvHops {
					px.Hops = env.Hops
				}
				//fmt.Println("=============", *px, len(px.Hops))
				//os.Exit(1)
			}
			if _, ok := deduplicate[service.Listen]; ok {
				fmt.Println("监听端口重复：", service.Listen)
				os.Exit(1)
			}
			deduplicate[service.Listen] = true
		}
	}
}