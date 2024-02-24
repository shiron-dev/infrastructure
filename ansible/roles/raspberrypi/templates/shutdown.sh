#!/bin/sh

# Copy /var/log to /var/log.old
cp -r /var/log /var/log.old/`date '+%Y%m%d%H%M'`
