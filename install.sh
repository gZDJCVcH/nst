#!/bin/sh
# openlist-cli 一键安装脚本
#
#   curl -fsSL https://raw.githubusercontent.com/OWNER/openlist-cli/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- -v v0.2.0
#   curl -fsSL .../install.sh | sh -s -- -d ~/bin
#
# 可选环境变量：
#   OPENLIST_CLI_DOWNLOAD_BASE  下载源（默认 https://github.com，可指向镜像）
#   BIN_DIR                     安装目录
set -eu

OWNER="OWNER"
REPO="openlist-cli"
BIN="openlist-cli"

VERSION="latest"
BIN_DIR="${BIN_DIR:-}"
DRY_RUN=""
TMP=""

usage() {
	cat <<EOF
用法: install.sh [选项]

  -v, --version <tag>   指定版本，如 v0.2.0（默认最新）
  -d, --dir <目录>       安装目录（默认 /usr/local/bin，不可写时用 ~/.local/bin）
      --dry-run          只下载并校验，不安装
  -h, --help             显示本帮助

示例:
  curl -fsSL https://raw.githubusercontent.com/$OWNER/$REPO/main/install.sh | sh
  curl -fsSL .../install.sh | sh -s -- -v v0.2.0 -d ~/bin
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	-v | --version)
		VERSION="$2"
		shift 2
		;;
	-d | --dir)
		BIN_DIR="$2"
		shift 2
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "未知参数: $1" >&2
		usage
		exit 1
		;;
	esac
done

BASE="${OPENLIST_CLI_DOWNLOAD_BASE:-https://github.com}"
BASE="${BASE%/}"

need() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "缺少依赖: $1" >&2
		exit 1
	}
}

fetch() { # fetch <url> <out>
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1"
	else
		echo "需要 curl 或 wget" >&2
		exit 1
	fi
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	FreeBSD) echo freebsd ;;
	*)
		echo "不支持的系统: $(uname -s)" >&2
		exit 1
		;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	armv7l | armv7 | armhf | armv6l) echo armv7 ;;
	*)
		echo "不支持的架构: $(uname -m)" >&2
		exit 1
		;;
	esac
}

resolve_tag() {
	if [ "$VERSION" != "latest" ]; then
		echo "$VERSION"
		return
	fi
	# 跟随一次 302 就能拿到 tag，不消耗 GitHub API 配额
	if command -v curl >/dev/null 2>&1; then
		tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
			"$BASE/$OWNER/$REPO/releases/latest" 2>/dev/null | sed 's#.*/##')
		if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
			echo "无法解析最新版本；请用 -v 指定版本号" >&2
			exit 1
		fi
		echo "$tag"
	else
		echo "解析最新版本需要 curl；请用 -v 指定版本号" >&2
		exit 1
	fi
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo ""
	fi
}

OS=$(detect_os)
ARCH=$(detect_arch)
TAG=$(resolve_tag)
VER=${TAG#v}

EXT="tar.gz"
if [ "$OS" = "windows" ]; then EXT="zip"; fi

ASSET="${BIN}_${VER}_${OS}_${ARCH}.${EXT}"
ASSET_URL="$BASE/$OWNER/$REPO/releases/download/$TAG/$ASSET"
SUMS_URL="$BASE/$OWNER/$REPO/releases/download/$TAG/checksums.txt"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

echo "→ 安装 ${BIN} ${TAG} (${OS}/${ARCH})"
fetch "$ASSET_URL" "$TMP/$ASSET"

echo "→ 校验 sha256"
fetch "$SUMS_URL" "$TMP/checksums.txt"
EXPECTED=$(grep " $ASSET\$" "$TMP/checksums.txt" | awk '{print $1}' || true)
if [ -z "$EXPECTED" ]; then
	echo "校验文件里没有 $ASSET，已中止（可能是恶意或被替换的下载源）" >&2
	exit 1
fi
ACTUAL=$(sha256_of "$TMP/$ASSET")
if [ -z "$ACTUAL" ]; then
	echo "警告：本机没有 sha256sum/shasum，跳过校验" >&2
elif [ "$EXPECTED" != "$ACTUAL" ]; then
	echo "sha256 校验失败：期望 $EXPECTED，实际 $ACTUAL" >&2
	exit 1
fi

if [ "$EXT" = "zip" ]; then
	need unzip
	unzip -q "$TMP/$ASSET" -d "$TMP"
else
	tar -xzf "$TMP/$ASSET" -C "$TMP"
fi

if [ -n "$DRY_RUN" ]; then
	echo "✓ 已下载并通过校验（dry-run，未安装）"
	exit 0
fi

if [ -z "$BIN_DIR" ]; then
	if [ -w /usr/local/bin ] 2>/dev/null; then
		BIN_DIR=/usr/local/bin
	else
		BIN_DIR="$HOME/.local/bin"
	fi
fi
mkdir -p "$BIN_DIR"

if [ -w "$BIN_DIR" ]; then
	install -m 0755 "$TMP/$BIN" "$BIN_DIR/$BIN"
else
	echo "需要写入 $BIN_DIR，尝试 sudo"
	sudo install -m 0755 "$TMP/$BIN" "$BIN_DIR/$BIN"
fi

echo "✓ 已安装到 $BIN_DIR/$BIN"
"$BIN_DIR/$BIN" version || true
case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*) echo "提示：$BIN_DIR 不在 PATH 中，加一行： export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
