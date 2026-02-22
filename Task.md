grasp-server 我希望包含两部分。一部分对外提供 mcp server，一部分作为 cli，即 cli 可以动态的挂载新的文件系统上去。当挂载上时，mcp server 会顺着文件系统的变更通知一路发送到 Agent，这样 Agent 能自发的去探索新的 fs

