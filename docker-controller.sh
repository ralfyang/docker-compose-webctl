#!/bin/bash

# 인자: docker-compose 파일의 절대경로
FILE="$1"
if [ -z "$FILE" ]; then
    echo "docker-compose 파일 경로가 필요합니다."
    exit 1
fi

# 파일이 있는 디렉토리로 이동
DIR=$(dirname "$FILE")
cd "$DIR" || exit 1

FILE_BASENAME=$(basename "$FILE")

echo "[docker-controller.sh] docker-compose 파일: $FILE_BASENAME 을(를) 사용하여 컨테이너 재시작..."
docker-compose -f "$FILE_BASENAME" down
sleep 2
docker-compose -f "$FILE_BASENAME" up -d
echo "[docker-controller.sh] 도커 시스템 재시작 완료."

