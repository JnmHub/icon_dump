

---

````markdown
# icon_dump

一个基于 Go 的小工具，用于在 **Windows** 系统下提取文件、文件夹或快捷方式（.lnk）的图标，并保存为 PNG 格式。

## ✨ 功能特点

- 支持提取 **文件 / 文件夹 / 快捷方式** 的图标  
- 自动解析 `.lnk`，获取真实目标文件的图标（不带快捷方式小箭头）  
- 输出为 PNG 文件，文件名与输入对象对应  
- 支持多种尺寸：  
  - `-s` → 16x16  
  - `-m` → 32x32  
  - `-l` → 48x48  
  - `-xl` → 256x256（若资源支持）

## 🚀 使用方法

1. 构建：
   ```bash
   go build -o icon_dump.exe
````

2. 执行：

   ```bash
   icon_dump.exe -l "C:\Windows\System32\notepad.exe"
   ```

   输出：

   ```
   已保存：notepad_48.png
   ```

3. 示例：

   ```bash
   # 提取快捷方式的真实图标（自动解析 .lnk）
   icon_dump.exe -xl "C:\Users\Public\Desktop\Notepad.lnk"
   ```

## ⚙️ 参数说明

* `-s` → 保存 16x16 图标
* `-m` → 保存 32x32 图标
* `-l` → 保存 48x48 图标（默认）
* `-xl` → 保存 256x256 图标（Vista+ 且资源内有大图标时才可用）

## 📦 系统要求

* Windows 7 及以上
* Go 1.21+

## 📜 许可证

[MIT License](LICENSE)

## 🔧 贡献

欢迎提交 **Pull Request** 改进此项目！

## 🌟 感谢

* [@gohugoio](https://github.com/gohugoio) - 图标提取代码灵感
* [@golang](https://golang.org) - 跨平台 Go 语言


