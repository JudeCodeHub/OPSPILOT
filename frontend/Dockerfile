# ---- build -------------------------------------------------------------------
FROM node:24.22.0-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# ---- runtime -----------------------------------------------------------------
# Non-root nginx listening on 8080. Files in /etc/nginx/templates are rendered
# with envsubst at startup (only variables defined in the environment).
FROM nginxinc/nginx-unprivileged:1.30.6-alpine
ENV API_UPSTREAM=http://gateway:8080
COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
