![GitHub version](https://img.shields.io/badge/version-v0.0.1-brightgreen.svg?logo=appveyor&longCache=true&style=flat)
![go report](https://goreportcard.com/badge/github.com/magicsong/porter)

# Porter

`Porter`是一款用于物理机部署的kubernetes的服务暴露插件，是[Kubesphere](https://kubesphere.io/)的一个子项目。

## 工作原理

该插件部署在集群中时，会与集群的边界路由器（三层交换机）建立BGP连接。每当集群中创建了带有特定注记的服务时，就会为该服务动态分配EIP，将EIP以辅助IP的形式绑定在Controller所在的节点主网卡上，然后创建路由，通过BGP将路由传导到公网（私网）中，使得外部能够访问这个服务。

## 如何使用

