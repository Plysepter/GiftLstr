#!/bin/sh
# Build versioned GiftLstr images and export them as tar archives.
#
# Usage:
#   sh scripts/release-docker.sh [VERSION] [--out DIR] [--only main|demo|all] [--no-latest]
#
# VERSION defaults to $VERSION, then to `git describe --tags --always --dirty`.
# The same VERSION tag is applied to both images, along with :latest unless
# --no-latest is given.
#
# Images:
#   giftlstr-giftlstr:<VERSION>       main app (Authentik OIDC)
#   giftlstr-giftlstr-demo:<VERSION>  demo/e2e build (VITE_E2E=true)
#
# Archives (default DIR: releases/):
#   giftlstr-<VERSION>.tar
#   giftlstr-demo-<VERSION>.tar
#
# Load a release with:
#   docker load -i releases/giftlstr-<VERSION>.tar
#
# Override the image names with MAIN_IMAGE / DEMO_IMAGE if needed.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

MAIN_IMAGE=${MAIN_IMAGE:-giftlstr-giftlstr}
DEMO_IMAGE=${DEMO_IMAGE:-giftlstr-giftlstr-demo}
VERSION=${VERSION:-}
OUT_DIR=${OUT_DIR:-releases}
ONLY=${ONLY:-all}
TAG_LATEST=${TAG_LATEST:-1}

usage() {
	sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	-h | --help)
		usage
		exit 0
		;;
	--out)
		[ $# -ge 2 ] || die "--out needs a directory"
		OUT_DIR=$2
		shift 2
		;;
	--out=*)
		OUT_DIR=${1#*=}
		shift
		;;
	--only)
		[ $# -ge 2 ] || die "--only needs main, demo, or all"
		ONLY=$2
		shift 2
		;;
	--only=*)
		ONLY=${1#*=}
		shift
		;;
	--no-latest)
		TAG_LATEST=0
		shift
		;;
	-*)
		die "unknown option: $1"
		;;
	*)
		[ -z "$VERSION" ] || die "version already set to $VERSION"
		VERSION=$1
		shift
		;;
	esac
done

case "$ONLY" in
main | demo | all) ;;
*) die "--only must be main, demo, or all" ;;
esac

if [ -z "$VERSION" ]; then
	if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
		VERSION=$(git describe --tags --always --dirty 2>/dev/null || true)
	fi
	[ -n "$VERSION" ] || VERSION=dev
fi

# Docker tags allow [A-Za-z0-9_][A-Za-z0-9_.-]{0,127}; the same restriction
# also keeps the archive names safe.
case "$VERSION" in
[A-Za-z0-9_]*) ;;
*) die "invalid version (must start with a letter, digit, or underscore): $VERSION" ;;
esac
case "$VERSION" in
*[!A-Za-z0-9_.-]*) die "invalid version (allowed: letters, digits, '.', '_', '-'): $VERSION" ;;
esac
[ ${#VERSION} -le 128 ] || die "version is longer than Docker's 128-character tag limit"

command -v docker >/dev/null 2>&1 || die "docker is not installed or not on PATH"

mkdir -p "$OUT_DIR"

# build_image expects IMAGE first, then any extra docker build args.
build_image() {
	image=$1
	shift
	echo "==> Building $image:$VERSION"
	docker build --file Dockerfile --tag "$image:$VERSION" "$@" .
	if [ "$TAG_LATEST" -eq 1 ]; then
		docker tag "$image:$VERSION" "$image:latest"
	fi
}

save_image() {
	image=$1
	archive=$2
	echo "==> Saving $archive"
	if [ "$TAG_LATEST" -eq 1 ]; then
		docker save --output "$archive" "$image:$VERSION" "$image:latest"
	else
		docker save --output "$archive" "$image:$VERSION"
	fi
}

if [ "$ONLY" = main ] || [ "$ONLY" = all ]; then
	build_image "$MAIN_IMAGE"
	save_image "$MAIN_IMAGE" "$OUT_DIR/giftlstr-$VERSION.tar"
fi

if [ "$ONLY" = demo ] || [ "$ONLY" = all ]; then
	build_image "$DEMO_IMAGE" --build-arg VITE_E2E=true
	save_image "$DEMO_IMAGE" "$OUT_DIR/giftlstr-demo-$VERSION.tar"
fi

echo
echo "Release $VERSION ready in $OUT_DIR/"
if [ "$ONLY" = main ] || [ "$ONLY" = all ]; then
	echo "  docker load -i $OUT_DIR/giftlstr-$VERSION.tar"
fi
if [ "$ONLY" = demo ] || [ "$ONLY" = all ]; then
	echo "  docker load -i $OUT_DIR/giftlstr-demo-$VERSION.tar"
fi
