#!/bin/bash

BRANCH=$(git rev-parse --abbrev-ref HEAD)
TAG=$(git describe --tags)
DEVELOP=develop

if [ "$BRANCH" = main ]; then
	echo $DEVELOP
else
        echo $TAG
fi
