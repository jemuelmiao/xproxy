### 作用说明
1、替换CRT、SwitchyOmega、hosts的配置，xproxy只需配置一个文件
2、方便前端开发时进行热更新，可以通过不同规则将后台请求、资源请求分开
### 配置说明

```
[[Envs]]
    # 环境名称
    Name = "基础环境测试"
    # 是否禁用环境
    Disable = false
    # 环境全局跳跃链，可以配置一个或多个，顺序决定跳跃顺序
    [[Envs.Hops]]
        Host = "ip:port"
        User = "跳板机用户"
        Password = "跳板机密码"
        # 本地sock代理地址
        Listen = "127.0.0.1:1080"
    # 需要访问的内网服务，可以配置一个或多个
    [[Envs.Services]]
	    # 服务名
        Name = "dig"
        # 服务是否禁用
        Disable = false
        # 本地浏览器访问地址
        Listen = ":10000"
        # 服务不同规则的代理，顺序重要
        [[Envs.Services.Proxys]]
            # 代理类型，http、sock5
            Type = "sock5"
            # 代理规则，匹配请求路径，标准正则表达式
            Rule = "^/(dag)|(cgi).*"
            # 服务所在局域网地址，例如本地前端热更新服务：127.0.0.1:8080，例如远程云环境内网服务：172.x.x.x:8090
            Host = "172.x.x.x:8090"
            # 是否使用全局跳跃链，不使用时可以配置本规则的私有跳跃链，配置方式参考全局的
            UseEnvHops = true
        [[Envs.Services.Proxys]]
            Type = "http"
            Rule = ".*"
            Host = "127.0.0.1:8080"
```
