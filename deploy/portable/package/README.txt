U Workers 服务端
=========================

升级：双击 Update.cmd，读取原安装设置，下载、校验并检查后输入 Y 升级。
先等任务空闲，用原安装 Stop.cmd 停止。取消不会改动安装；失败请保留显示的
下载和备份目录，按下载目录中的 UPGRADE.md 恢复。
v0.2.5 首次使用需取得并校验外置 UWorkers-Fetch-bootstrap.zip，旧安装没有此入口。
客户端首次连接用同版本 Client-Kit 的 Connect.cmd；换新版先用 Import.cmd 导入旧包。
Update 用于同机原地升级。新电脑首次安装请用未安装的完整包运行 Setup.cmd，
换服务器的数据迁移另行安排；不要把正在使用的安装目录当发布包。

Everything in this folder is the server: PostgreSQL, the API, the web app,
LAN HTTPS with its own private root certificate, the desktop sign-in account
and a local opencode runtime. No administrator rights and no internet
connection are needed.

First run
---------
1. Copy this whole folder to a writable place (for example
   %LOCALAPPDATA%\U Workers Portable).
2. Double-click Setup.cmd. The first start takes a minute or two.
3. The browser opens the sign-in page. Use the user name printed by Setup
   and the password stored in data\local-login.txt.

Everyday
--------
  Start.cmd      start the server (also starts automatically at sign-in)
  Stop.cmd       stop it; data is kept
  Status.cmd     what is running, which ports, where the root certificate is
  Update.cmd     download, verify and confirm an upgrade using existing settings
  Uninstall.cmd  remove the service, the start-on-login item and the browser
                 trust; add -PurgeData to delete the database and uploads

Other machines
--------------
The LAN entry point (https://<this-machine-LAN-IP>:13443) only listens when a
Windows Firewall inbound rule already covers this installation's caddy.exe, or
when Setup could add one (that needs administrator rights). Otherwise Setup
prints "LAN access is off" and Caddy listens on 127.0.0.1 only. That is
deliberate: the alternative is the Windows Firewall prompt on every start, and
the person at a standard-user machine cannot answer it. To grant LAN access
once, run Setup.cmd as an administrator, or ask IT for an inbound TCP 13443
rule for payload\runtime\caddy\caddy.exe from the LAN subnet. Status.cmd shows
the current state on its "LAN entry" line.

1. Copy root-ca.crt (in this folder) to the other Windows machine.
2. Run the u-workers-setup EXE from payload\installer (or build a fresh one
   with Build-Installer.cmd).
3. Run:  u-workers connect --server-url https://<this-machine-LAN-IP>:13443
        --root-ca .\root-ca.crt

Building the installer from this folder
---------------------------------------
No internet needed. Run Build-Installer.cmd; it uses the bundled Go
toolchain, module cache and source snapshot, and writes the EXE to dist\.

Folder map
----------
  payload\release     prebuilt API binaries, migrations and web app
  payload\bin         u-workers CLI (installed daemon binary)
  payload\runtime     node.exe, PostgreSQL, Caddy, opencode
  payload\source      source snapshot used to rebuild the installer
  payload\installer   the prebuilt u-workers-setup EXE for other machines
  tools\              Go toolchain and module cache for offline rebuilds
  data\               database, uploads, logs, backups, install report
  runtime\            staged server runtime (PostgreSQL, node, runner)
  releases\           staged releases; upgrade keeps the previous one
  https\              Caddyfile, private CA data and the public root CA
