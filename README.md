# qtum-adapter

本项目适配了openwallet.AssetsAdapter接口，给应用提供了底层的区块链协议支持。

## 如何测试

openwtester包下的测试用例已经集成了openwallet钱包体系，创建conf文件，新建QTUM.ini文件，编辑如下内容：


```ini

# RPC Server Type，0: CoreWallet RPC; 1: Explorer API
rpcServerType = 1
# Qtum server url
apiURL = "http://127.0.0.1:20007/qtum-insight-api/"
# RPC Authentication Username
rpcUser = "test"
# RPC Authentication Password
rpcPassword = "test1234"
# Is network test?
isTestNet = false

```
