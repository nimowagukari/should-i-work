FROM golang:1.26.2-trixie

ARG UID=1000
ARG GID=1000

# 一般ユーザの作成
RUN groupadd -g ${GID} golang && \
    useradd -m -s /bin/bash -g golang golang

# OS パッケージのインストール
RUN apt-get update && apt-get install -y \
        less \
        locales \
        vim \
        zip && \
    rm -rf /var/lib/apt/lists/*

# 日本語フォントの設定
RUN sed -ri -e "s/^# ja_JP.UTF-8/ja_JP.UTF-8/g" /etc/locale.gen && \
    locale-gen && \
    update-locale LANG=ja_JP.UTF-8 && \
    echo 'export LANG=ja_JP.utf8' >> ~/.bashrc && \
    echo 'export LANG=ja_JP.utf8' >> ~golang/.bashrc

# aws-lambda-rie インストール
RUN curl -Lo /usr/local/bin/aws-lambda-rie https://github.com/aws/aws-lambda-runtime-interface-emulator/releases/latest/download/aws-lambda-rie && \
    chmod +x /usr/local/bin/aws-lambda-rie

WORKDIR /workspaces/should-i-work
USER golang
