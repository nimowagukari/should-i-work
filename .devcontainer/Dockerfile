FROM golang:1.26.2-trixie AS base

ARG UID=1000
ARG GID=1000

SHELL ["/bin/bash", "-c"]

# 一般ユーザの作成
RUN groupadd -g ${GID} golang && \
    useradd -m -s /bin/bash -g golang golang

# OS パッケージのインストール
RUN apt-get update && apt-get install -y \
    gh \
    less \
    locales \
    vim \
    zip \
    && rm -rf /var/lib/apt/lists/*

# 日本語フォントの設定
RUN sed -ri -e "s/^# ja_JP.UTF-8/ja_JP.UTF-8/g" /etc/locale.gen && \
    locale-gen && \
    update-locale LANG=ja_JP.UTF-8 && \
    echo 'export LANG=ja_JP.utf8' >> ~/.bashrc && \
    echo 'export LANG=ja_JP.utf8' >> ~golang/.bashrc


# 作業ディレクトリの作成と権限設定
RUN mkdir -p /workspaces/should-i-work \
    && chown -R golang:golang /workspaces/should-i-work
WORKDIR /workspaces/should-i-work




# 開発用
FROM base AS dev

# aws-lambda-rie インストール
RUN curl -Lo /usr/local/bin/aws-lambda-rie https://github.com/aws/aws-lambda-runtime-interface-emulator/releases/latest/download/aws-lambda-rie && \
    chmod +x /usr/local/bin/aws-lambda-rie

USER golang

## デバックに必要な golang パッケージのインストール
RUN go install golang.org/x/tools/gopls@latest && \
    go install github.com/go-delve/delve/cmd/dlv@latest && \
    go install github.com/google/yamlfmt/cmd/yamlfmt@latest

# Claude 設定
RUN mkdir -p /home/golang/.claude
