# 关于kube-loadbalancer-controller 负载均衡器插件

## kube-loadbalancer-controller的功能
　　kube-loadbalancer-controller用于在barematel方式部署的k8s集群中将服务暴露给集群外部。

## kube-loadbalancer-controller的工作原理

　　kube-loadbalancer-controller插件部署在集群中后会和集群边界交换机之间建立BGP peer，插件会监听集群中service 和 endpoint变化。   
当感知到集群中创建了LB类型的service时，会在工作节点上自动配置针对该服务的引流规则，并通过监听endpoint获取该service下的pod所在    
的node IP，以BGP等价路由的形式将这些node IP发布给边界路由器。这样，当外部对该servcie的请求达到交换机时，交换机能将该请求转至集    
群中相应的node节点上，node节点根据引流规则会捕获该请求转本地处理，处理完成后将响应返回给交换机，从而实现了对外部访问的支持。    
　　同时由于插件会监控集群中endpoint变化，当集群中有servcie上下线、pod漂移等情况时，插件能动态调整相应路由并通过BGP协议同步到交换机。
    
#  单Master节点部署 kube-loadbalancer-controller 负载均衡器插件

## 安装前提
1. 硬件要求   
 无
2. 软件要求   
 需要先具有一个bare-metal的k8s集群,集群版本要求k8s1.10及以上版本。

## 部署方法
  部署分为三部分：在主节点上部署loadbalancer-controller、在所有从节点上部署loadbalancer-daemon以及配置外部交换机
 - 主节点上部署loadbalancer-controller，用于监听集群中LB类型servcie，向外发布访问这些服务的路由。
 - 从节点上部署loadbalancer-daemonset，用于配置引流规则，将外部请求转到节点本地处理。
 - 配置外部交换机，以实现和loadbalancer-controller的BGP连接，并接受其发出的路由。

## 在主节点部署loadbalancer-controller
1. BGP peer本端Port获取   
   - 若集群CNI插件为flannel方式，则该BGP peer本端可以用BGP协议默认的179端口；
   - 若CNI插件为calico方式，则挑选一个宿主机上未占用端口作为BGP peer本端端口，如1790。（需先确认集群防火墙没有禁用该端口）
    
2. 修改configmap.yaml中kube-gobgp-cfg下的config_bgp.toml如下几项配置。其中：
   - port为第1步中获取的端口
   - neighbor-address为外部交换机IP
   - as、peer-as根据实际规划填写

3. 修改configmap.yaml中kube-iprule-cfg下的data值。其中:
   - SWITCH为必选项，填外部交换机IP
   - IN_ROUTING_TABLE为可选项，可用默认值，也可以改为其他路由表
   - DEPLOY_MODEL为必选项，其中“barematel”表示k8s集群物理部署，“test”表示在云平台上部署。默认为“barematel”
   > **注**：
        *1. 生成环境采用实际物理部署，即此处填“barematel”*   
        *2. 在青云平台下测试时，此处填“test”，会将node节点默认网关改为switch IP，此时VPC端口转发方式将不可用，只能通过网页端登录node节点*
     
4. 运行configmap.yaml
   ```    
   #kubectl create -f configmap.yaml
   ```
   
5. 运行rbac.yaml为loadbalancer-controller、loadbalancer-daemon的运行创建serivice account
   ```    
   #kubectl create -f rbac.yaml
   ```
6. 运行loadbalancer-controller.yaml，将loadbalancer-controller部署到主节点
   ```
   #kubectl create -f loadbalancer-controller.yaml
   ``` 

7. 运行loadbalancer-daemonset.yaml，在各个从节点上部署loadbalancer-daemonset
   ```
   kubectl create -f loadbalancer-daemonset.yaml
   ```

## 配置外部交换机
   外部交换机主要是配置与loadbalancer-controller之间建立BGP peer。下面以H3C s6800交换机为例讲述下配置过程：
### H3C s6800交换机配置   
   ```
   bgp 65001   
   graceful-restart      
   router-id 192.168.1.4   
   peer 192.168.1.5 as-number 65000   
   balance eibgp 8
   ```
   由于实际测试场景下，可能不一定具备物理交换机，下面再讲解在主机上通过bird来模拟交换机时的操作过程。   
### Bird模拟交换机配置
   测试环境下，可在物理机或虚拟机上安装Bird模拟交换机（要求和master节点连通），以ubuntu16.04上操作过程为例： 
   - 安装bird1.5
     ```
     $sudo apt-get update 
     $sudo apt-get install bird  
     ```
   - 修改配置文件/etc/bird/bird.conf，添加如下同loadbalancer-controller建立BGP peer的配置信息
     ```
        protocol bgp xenial100 {   
            description "192.168.1.4"; ##填本交换机ID   
            local as 65001; ##填本地AS域   
            neighbor 192.168.1.5 port 1790 as 65000; ##填master节点IP和 AS域   
            source address 192.168.1.4; ##填本交换机IP    
            import all;   
            export all;
        }
     ```
   - 执行“sudo /etc/init.d/bird start”，启动Bird   
   - 执行“sudo birdc configure”，使配置文件生效
   
## 验证部署是否成功
1. 验证loadbalancer-controller是否部署成功
   - 执行"kubectl get pods --namespace=kube-system"查看loadbalancer-controller对应的pod是否正常启动
   - 在主节点上执行"ps -aef |grep bgp",查看gobgp进程是否正常启动
   - 进入loadbalancer-controller对应的pod，执行“/bin/gobgp neighbor”,查看与外部交换机的bgp peer是否成功
   
2.  验证loadbalancer-daemonset是否部署成功
   - 执行"kubectl get pods --namespace=kube-system"查看loadbalancer-daemonset对应的pod是否正常启动   
   
3. 部署一个LB类型的service，定义service的yaml中，spec.type字段填为LoadBalancer，spec.externalIPs字段至少填一个实际的EIP

4. 查看主节点上生成的BGP路由
   - 在loadbalancer pod上执行“/bin/gobgp global rib -a ipv4”,查看BGP本端获取的对应上述EIP的路由,是否与该service下所有pod所在的NODE IP一致。  
   实际例子如下：

       ```
        root@i-oruhcy5j:~# kubectl get svc -o wide   
        NAME　　　　　　TYPE　　　　　CLUSTER-IP　　　EXTERNAL-IP　　　PORT(S)　　　　　AGE　　　SELECTOR   
        kubernetes　　ClusterIP　　　10.96.0.1　　　<none>　　　443/TCP　　　　　　　14d　　　　<none>   
        loadbalancer　ClusterIP　　　　None　　　　　　<none>　　　　　<none>　　　　　13h　　　　app=loadbalancer    
        myweb　　　　LoadBalancer　　　10.96.209.55　　139.198.190.117　8080:32767/TCP　　13h　　　　app=myweb    
     
        root@i-oruhcy5j:~# kubectl get pod   -o wide   
        NAME　　　　　　　　　　　　　　　　READY　　　　　STATUS　　　　　RESTARTS　　　AGE　　　　　IP　　　　　　　　　NODE   
        myweb-8jt6s　　　　　　　　　　　1/1　　　　　　Running　　　　　　　0　　　　　　13h　　　10.244.31.20　　　i-rnqf0xcu   
        myweb-fgtfz　　　　　　　　　　　1/1　　　　　　Running　　　　　　　0　　　　　　13h　　　10.244.230.18　　i-9u2mlxrn  
      
        root@i-oruhcy5j:~# /bin/gobgp global rib -a ipv4      
         Network　　　　　　　　　　　　　　　　Next Hop　　　　　　AS_PATH　　　　　Age　　　　　　　　Attrs    
         *> 139.198.190.117/32　　　　　192.168.1.14　　　　　　　　　　　　　13:01:42　　　　[{Origin: ?}]   
         *  139.198.190.117/32　　　　　192.168.1.11　　　　　　　　　　　　　13:01:42　　　　[{Origin: ?}]   
     
        root@i-oruhcy5j:~# cat /etc/hosts      
        127.0.0.1　　　　　localhost    
        ff02::1　　ip6-allnodes      
        ff02::2　　ip6-allrouters    
        192.168.1.5　　i-oruhcy5j    
        192.168.1.7　　i-5mtgih8e     
        192.168.1.11　　i-9u2mlxrn      
        192.168.1.14　　i-rnqf0xcu    
       ```
5. 查看从节点上生成的ip rule规则
   ```
   ip rule list
   ```

6. 查看交换机上的BGP连接及路由
   - H3C s6800交换机上查看BGP连接
   ```
   display current-configuration configuration bgp
   ```
   - H3C s6800交换机上查看收到的BGP路由
   ```
   display bgp routing-table ipv4 peer 192.168.1.5 received-routes
   ```
   - Bird模拟交换机上查看BGP连接
   ```
   sudo birdc show protocols
   ```
   - Bird模拟交换机上查看BGP路由
   ```
   sudo birdc show route all
   ```
   
      > **注**：*通过apt-get方式只能安装1.5版本的bird，该版本没有BGP mutilpath功能，在收到的同一目的地址下的多条等价路由中只会保留一条路由信息。*  
      > *若要在模拟交换机上保留同一目的地址下的所有ECMP路由，需安装bird 1.6以上版本，并在配置文件中开启add-path功能，参考如下：*  
        *https://bird.network.cz/?get_doc&v=20&f=bird-1.html#ss1.2*  
        *https://bird.network.cz/?get_doc&v=16&f=bird-6.html#ss6.3*

7. 集群外部通过浏览器等方式实际访问服务   
    当用Bird模拟交换机时，可在模拟交换机上进行如下配置，以支持在集群外部浏览器等方式访问集群中某lb类型服务  
    - 给模拟交换机绑定eip，该eip即为集群中该lb类型服务所分配的eip。
    
        > **注**：
        *1. 模拟交换机上绑定eip后，会新生成一张网卡，不要给这张网卡配ip地址;*   
        *2. 如果在青云的云平台中进行测试，需要在防火墙上打开服务对外访问的端口，绑定过程可参考
          https://docs.qingcloud.com/product/network/eip#%E4%BD%BF%E7%94%A8%E5%86%85%E9%83%A8%E7%BB%91%E5%AE%9A%E5%85%AC%E7%BD%91-ip*    
        *3. 在青云平台下，私有网络内的主机绑定了 EIP 后，外网访问将使用 EIP 作为进出网关。如果同时配置了 VPC 的端口转发规则，端口转发规则将不再有效。   
         此种情况下原有VPC端口转发方式连接交换机将会断开，只能通过网页端继续登录该模拟交换机*
    - 在模拟交换机上配置如下路由规则，其中139.198.xx.xx即为绑定给模拟交换机的EIP，192.168.xx.xx为模拟交换机eth0口IP：
        ```
        #ip route replace default dev eth1
        #ip route replace 139.198.xx.xx via 192.168.xx.xx dev eth0

        ``` 
    - 打开模拟交换机的端口转发并关闭包过滤  
        ```
        sysctl -w net.ipv4.ip_forward=1   
        sysctl -w net.ipv4.conf.all.rp_filter=0    
        sysctl -w net.ipv4.conf.eth1.rp_filter=0
        sysctl -w net.ipv4.conf.eth0.rp_filter=0
        ```
8. 在集群中新增\删除LB类型service 观察模拟交换机上BGP路由变化及node节点ip rule变化

9. 集群中新增一个node节点时，观察该节点是否会自动同步集群中现有LB类型service对应的引流规则

10. 集群中迁移一个LB类型service所属的pod，观察交换机上BGP路由调整是否正确。
   ```
   kubectl drain <node name>   
   ```

#  多Master节点部署 kube-loadbalancer-controller 负载均衡器插件
   多Master节点部署负载均衡器插件过程和单节点时部署过程大同小异，以在3个master节点的集群中部署2 replica 负载均衡器插件为例，不同点如下：   
   1.部署loadbalancer-controller时,loadbalancer-controller.yaml中StatefulSet里replica改为2(实际部署时replica数应等于或小于master节点个数)   
   2.在交换机上将3个master节点均配置为BGP peer对端。   
   - 以Bird模拟交换机为例，/etc/bird/bird.conf中配置信息如下：
     ```
        protocol bgp xenial100 {   
            description "192.168.1.4";   
            local as 65001;   
            neighbor 192.168.1.5 port 1790 as 65000;    
            source address 192.168.1.4;   
            import all;   
            export all;   
        }   
        
        protocol bgp xenial101 {   
            description "192.168.1.4";    
            local as 65001;   
            neighbor 192.168.1.6 port 1790 as 65000;    
            source address 192.168.1.4;    
            import all;   
            export all;   
        }   
        
        protocol bgp xenial102 {   
            description "192.168.1.4";    
            local as 65001;   
            neighbor 192.168.1.7 port 1790 as 65000;     
            source address 192.168.1.4;    
            import all;   
            export all;   
        }
     ```
   
#  如何使用 kube-loadbalancer-controller 负载均衡器插件

　　要使用kube-loadbalancer-controller 负载均衡器插件只需要在部署服务时，将服务定义为LB类型，并指定EIP，yaml中定义如下：
> spec.type字段填为"LoadBalancer"   
> spec.externalIPs字段至少填一个实际的EIP
