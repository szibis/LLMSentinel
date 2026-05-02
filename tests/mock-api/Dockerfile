FROM node:20-alpine

WORKDIR /app

COPY package.json package-lock.json ./

RUN npm ci --only=production

COPY server.js .

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/health', (r) => { if (r.statusCode !== 200) throw new Error(); })"

CMD ["node", "server.js"]
