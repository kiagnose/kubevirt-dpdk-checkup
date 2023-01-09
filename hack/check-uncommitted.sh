#!/usr/bin/env bash

if [[ -n "$(git status --porcelain)" ]] ; then
    echo "You have Uncommitted changes. Please commit the changes"
    git status --porcelain
    git diff
    exit 1
fi
