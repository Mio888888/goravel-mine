FROM node:22-alpine3.22 AS build

WORKDIR /opt/www

RUN test "$(yarn --version)" = "1.22.22"

COPY package.json yarn.lock .npmrc ./
RUN yarn install --frozen-lockfile

COPY . /opt/www/

ARG MINE_NODE_ENV=production

RUN echo "MINE_NODE_ENV=$MINE_NODE_ENV"

RUN if [ "$MINE_NODE_ENV" = "development" ]; then yarn build --mode development; fi && \
    if [ "$MINE_NODE_ENV" = "production" ]; then yarn build --mode production; fi


FROM alpine:3.22 AS production

RUN apk add --no-cache nginx nginx-mod-http-brotli \
    && addgroup -S mineweb -g 10002 \
    && adduser -S -D -H -u 10002 -G mineweb mineweb \
    && mkdir -p /usr/share/nginx/html \
        /etc/nginx/conf.d \
        /etc/nginx/snippets \
        /var/lib/nginx \
        /var/log/nginx \
        /tmp/nginx/client_temp \
        /tmp/nginx/proxy_temp \
        /tmp/nginx/fastcgi_temp \
        /tmp/nginx/uwsgi_temp \
        /tmp/nginx/scgi_temp \
    && chown -R mineweb:mineweb /usr/share/nginx/html /tmp/nginx /var/lib/nginx /var/log/nginx

COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/conf.d/ /etc/nginx/conf.d/
COPY nginx/snippets/ /etc/nginx/snippets/
COPY --from=build --chown=mineweb:mineweb /opt/www/dist /usr/share/nginx/html

USER mineweb

EXPOSE 8080

STOPSIGNAL SIGQUIT

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/ >/dev/null 2>&1 || exit 1

CMD ["nginx", "-g", "daemon off;"]
