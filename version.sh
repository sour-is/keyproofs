#!/bin/bash

# Increment a version string using Semantic Versioning (SemVer) terminology.

# Parse command line options.

case $BUMP in
  current ) ;;
  major   ) major=true;;
  minor   ) minor=true;;
  patch   ) patch=true;;
esac

version=$(git describe --tags `git rev-list --tags --max-count=1 2> /dev/null` 2> /dev/null|cut -b2-)

# Build array from version string.

a=( ${version//./ } )

# If version string is missing or has the wrong number of members, show usage message.

if [ ${#a[@]} -ne 3 ]
then
  version=0.0.0
  a=( ${version//./ } )
fi

# Increment version numbers as requested.

if [ ! -z $major ]
then
  ((a[0]++))
  a[1]=0
  a[2]=0
fi

if [ ! -z $minor ]
then
  ((a[1]++))
  a[2]=0
fi

if [ ! -z $patch ]
then
  ((a[2]++))
fi

if git status --porcelain >/dev/null
then
  echo "${a[0]}.${a[1]}.${a[2]}"
else
  echo "${a[0]}.${a[1]}.${a[2]}-dirty"
fi