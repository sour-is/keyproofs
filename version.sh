#!/bin/bash

# Increment a version string using Semantic Versioning (SemVer) terminology.
# Parse command line options.
BUMP="${BUMP:="$1"}"

case $BUMP in
  current ) ;;
  major   ) major=true;;
  minor   ) minor=true;;
  patch   ) patch=true;;
esac

version=$(git describe --tags "$(git rev-list --tags --max-count=1 2> /dev/null)" 2> /dev/null|cut -b2-)

# Build array from version string.

IFS="." read -r -a a <<< "$version"

# If version string is missing or has the wrong number of members, show usage message.

if [ ${#a[@]} -ne 3 ]
then
  version=0.0.0
  IFS="." read -r -a a <<< "$version"
fi

# Increment version numbers as requested.

if [ -n "$major" ]
then
  ((a[0]++))
  a[1]=0
  a[2]=0
fi

if [ -n "$minor" ]
then
  ((a[1]++))
  a[2]=0
fi

if [ -n "$patch" ]
then
  ((a[2]++))
fi

if git status --porcelain >/dev/null
then
  echo "v${a[0]}.${a[1]}.${a[2]}"
else
  echo "v${a[0]}.${a[1]}.${a[2]}-dirty"
fi