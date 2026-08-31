# CPA 商店上架准备(待提交)

> 状态:**轮子已备好,时机成熟再动**。本目录是提交官方插件商店
> (`router-for-me/CLIProxyAPI-Plugins-Store`)前的全部准备物与检查清单。

## 文件清单

| 文件 | 用途 |
|---|---|
| `store-registry-entry.json` | 待合并进官方 `registry.json` 的 usage-lens 条目 |
| (仓库根) `assets/logo.png` | 商店 logo(GitHub raw URL 引用) |
| (仓库根) `Makefile` + `dist/` | `make release` 产出平台 zip + `checksums.txt` |
| (仓库根) `LICENSE` / `README.md` / `VERSION` | 商店质量基线 |

## 官方规范要点(2026-08 核对 router-for-me/CLIProxyAPI-Plugins-Store README)

- 商店仓库只维护 `registry.json`,插件二进制/校验和/release 存作者自己的仓库。
- 条目字段:必需 `id / name / description / author / repository`;可选
  `version / logo / homepage / license / tags`。
- 校验:`schema_version=1`;`id` 唯一且只含 ASCII 字母/数字/`.`/`_`/`-`;
  `version` 不以 `v` 开头;`repository` 必须精确 `https://github.com/{owner}/{repo}`。
- **版本以 release tag 为准**:tag 必须 `v<点分数字>`,如 `v0.1.0`。
- Release 资产命名:`<id>_<version>_<goos>_<goarch>.zip` + `checksums.txt`
  (sha256sum 格式);zip 根目录直放动态库(`usage-lens.so`),嵌套/绝对路径/zip-slip/文件名不匹配会被安装器拒绝。

## 提交动作(时机成熟时执行,顺序)

1. **发 GitHub release**:tag `v0.1.0`,随附 `dist/` 下全部
   `usage-lens_0.1.0_*.zip` + `checksums.txt`(先 `make release` 重新生成,
   确保 checksums 与 release 资产一致)。
2. **产物自检**:
   ```bash
   make release
   unzip -l dist/usage-lens_0.1.0_linux_amd64.zip   # 根目录应只有 usage-lens.so
   cd dist && sha256sum -c checksums.txt
   ```
3. **提 PR 到 `router-for-me/CLIProxyAPI-Plugins-Store`**:只改 `registry.json`,
   把 `store-registry-entry.json` 内容嵌进 `plugins` 数组(唯一改动)。
   PR 描述附:
   - 仓库 URL:https://github.com/hex-ci/cpa-plugin-usage-lens
   - release tag:`v0.1.0`(含 zip/checksums 资产链接)
   - 能力一句话:零独立服务的用量分析面板(请求/Token/成本/缓存/延迟,按模型与 API Key)。
4. 合并后即上架完成;后续更新只需发新 release(tag `v0.2.0` 等),registry 不用再动。

## 注意事项

- 上架前先确认插件 metadata 完整(本项目 `usage-lens.so` 的注册元数据
  Name/Version/Author/GitHubRepository/Logo/ConfigFields)。
- 商店目录:`/tmp/cpa-store`(官方仓库 clone,只读参考)。
- 多平台产物:当前机器无交叉 C 工具链,`make release` 只出 linux/amd64;
  有 aarch64/OSX 工具链的环境跑同一命令即补全其余平台。