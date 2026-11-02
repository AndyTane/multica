# 第二台机器接入（Windows）

目标：一台没装过 U Workers 的 Windows 电脑，装一个安装包，一条命令连上已经部署好的服务端并启动守护进程；局域网自签证书不再报错。

适用场景：服务端按 [Windows 原生自托管](windows-native.md) + [deploy/windows-https](../deploy/windows-https/README.md) 跑在台式机上，HTTPS 入口用的是 Caddy 本地 CA。公开证书的服务端不需要 `--root-ca` 这一步。

## 3 步

### 1. 在台式机准备两个文件

```powershell
# 根证书：导出到 <deployment>\https\public\root-ca.crt，并打印 SHA-256
pwsh -File deploy\windows-https\caddy-windows.ps1 -Action ExportRoot -Root D:\UWorkers\clawsta80\native

# 安装包：单文件 exe，内含当前版本的 CLI（u-workers 和 multica 两个命令一起装）
pwsh -File scripts\build-windows-installer.ps1
# 产物：dist\u-workers-setup-<version>-windows-amd64.exe
```

把 `root-ca.crt` 和 `u-workers-setup-*.exe` 拷到新机器（共享目录、U 盘、IM 传文件都行）。

### 2. 在新机器上安装

```powershell
.\u-workers-setup-<version>-windows-amd64.exe
```

安装到 `%LOCALAPPDATA%\U Workers\bin`，把该目录加入当前用户 PATH，并在「添加/删除程序」里登记。不需要管理员权限。重开一个终端后：

```powershell
u-workers version   # 当前命令
multica version     # 旧命令，同一份二进制
```

### 3. 一条命令连接

```powershell
u-workers connect --server-url https://192.168.3.49:13443 --root-ca .\root-ca.crt
```

这一条命令依次做四件事：

1. 用 `root-ca.crt` 验证服务端当前证书确实由它签发。不匹配就直接失败，什么都不写入。
2. 把根证书存到 `~/.u-workers/trust/root-ca.pem`，CLI 和守护进程之后所有请求都带上它；**不关闭任何证书校验**，只是多一个信任锚。
3. Windows 会弹一次「安全警告」确认框，点「是 / Yes」。这样浏览器打开看板也不再报证书错误。不想让浏览器信任就加 `--skip-browser-trust`，CLI 和守护进程照常工作。
4. 打开浏览器登录（想完全无浏览器就加 `--token mul_...`），保存服务端地址，启动守护进程，等它在看板里注册出这台机器的运行时。

成功时输出：

```text
✓ Connected to https://192.168.3.49:13443
  app:      https://192.168.3.49:13443
  daemon:   running (pid 12345)
  runtimes: 1 online
```

回到看板的「运行时」页面就能看到这台机器。

## 之后常用

| 想做什么 | 命令 |
| --- | --- |
| 看守护进程状态 / 日志 | `u-workers daemon status` · `u-workers daemon logs -f` |
| 换服务端或重新信任 | 重跑 `u-workers connect ...` |
| 清掉已信任的根证书 | `u-workers config set root_ca_file ""`，然后 `u-workers daemon restart` |
| 卸载 | 用当初的安装包：`u-workers-setup-<version>-windows-amd64.exe --uninstall` |

## 边界与排错

- **机器上还没有智能体 CLI**：守护进程会在线，但注册不出运行时。装一个（`codex`、`claude`、`dsh` 等）后 `u-workers daemon restart`，或在守护进程运行中等待几分钟，它自己会发现。
- **`connect` 报 certificate / not issued by**：`root-ca.crt` 和服务端当前证书不是一对。重新在台式机执行第 1 步导出；确认访问的是同一个入口地址。
- **`--trust-browser` 的确认框一直没点**：3 分钟后 `connect` 会报超时退出，CLI 和守护进程的信任不受影响（证书文件已保存），之后可用 `deploy/windows-https/import-client-root.ps1` 单独给浏览器装。
- **没有安装包可用**：在台式机 checkout 里执行 `pwsh -File scripts\build-windows-installer.ps1 -Version <版本>` 生成；单文件 exe 的大小约 21 MB。
- **旧流程仍然可用**：`u-workers setup self-host --server-url ... --app-url ...` 是改名前的等价命令，证书需要按旧方式手动装。
