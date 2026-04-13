import platform
import os

print("=" * 50)
print("操作系统信息")
print("=" * 50)

# 操作系统基本信息
print(f"\n操作系统: {platform.system()}")
print(f"操作系统版本: {platform.release()}")
print(f"详细版本: {platform.version()}")
print(f"系统架构: {platform.machine()}")
print(f"处理器: {platform.processor()}")
print(f"主机名: {platform.node()}")

print("\n" + "=" * 50)
print("Python环境信息")
print("=" * 50)
print(f"Python版本: {platform.python_version()}")
print(f"Python实现: {platform.python_implementation()}")
print(f"Python编译器: {platform.python_compiler()}")

print("\n" + "=" * 50)
print("网络信息")
print("=" * 50)
print(f"主机名: {os.uname().nodename}")

print("\n" + "=" * 50)
print("当前目录和用户")
print("=" * 50)
print(f"当前工作目录: {os.getcwd()}")
print(f"当前用户: {os.getenv('USER', 'Unknown')}")
print(f"家目录: {os.path.expanduser('~')}")
