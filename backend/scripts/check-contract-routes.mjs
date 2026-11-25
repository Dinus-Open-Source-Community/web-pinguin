import fs from 'node:fs';

const router = fs.readFileSync(new URL('../internal/http/router.go', import.meta.url), 'utf8');
const spec = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const expected = new Set();
const normalize = (path) => path.replace(/:([A-Za-z0-9_]+)/g, '{$1}');
for (const match of router.matchAll(/\b(engine|api|admin|adminStatus)\.(GET|POST|PATCH|PUT|DELETE)\("([^"]+)"/g)) {
  const [, group, method, declaredPath] = match;
  let route = normalize(declaredPath);
  if (group === 'api') route = `/api/v1${route}`;
  if (group === 'admin' || group === 'adminStatus') route = `/api/v1/admin${route}`;
  const paths = route.endsWith('/{action}')
    ? ['publish', 'close', 'archive'].map((action) => route.replace('/{action}', `/${action}`))
    : [route];
  for (const path of paths) expected.add(`${method.toLowerCase()} ${path}`);
}
const documented = new Set();
for (const [path, operations] of Object.entries(spec.paths ?? {})) {
  const pathServers = operations.servers ?? spec.servers ?? [{ url: '/' }];
  const serverPath = (pathServers[0]?.url ?? '/').replace(/^https?:\/\/[^/]+/i, '').replace(/\/$/, '');
  const runtimePath = `${serverPath}/${path.replace(/^\//, '')}`.replace(/\/{2,}/g, '/');
  for (const method of Object.keys(operations)) {
    if (['get', 'post', 'put', 'patch', 'delete', 'options', 'head'].includes(method)) documented.add(`${method} ${runtimePath}`);
  }
}
const missing = [...expected].filter((item) => !documented.has(item));
const stale = [...documented].filter((item) => !expected.has(item));
const contractErrors = [];
const authScheme = spec.components?.securitySchemes?.adminAuth;
if (authScheme?.type !== 'http' || authScheme?.scheme !== 'bearer' || authScheme?.bearerFormat !== 'JWT') {
  contractErrors.push('components.securitySchemes.adminAuth must declare HTTP bearer JWT authentication');
}
for (const [path, operations] of Object.entries(spec.paths ?? {})) {
  for (const [method, operation] of Object.entries(operations)) {
    if (!['get', 'post', 'put', 'patch', 'delete', 'options', 'head'].includes(method)) continue;
    if (!operation.responses || Object.keys(operation.responses).length === 0) {
      contractErrors.push(`${method.toUpperCase()} ${path} has no documented response`);
    }
    if (path.startsWith('/admin/')) {
      const security = operation.security ?? spec.security ?? [];
      if (!security.some((requirement) => Object.hasOwn(requirement, 'adminAuth'))) {
        contractErrors.push(`${method.toUpperCase()} ${path} has no adminAuth security requirement`);
      }
      for (const status of ['401', '403']) {
        if (!Object.hasOwn(operation.responses, status)) {
          contractErrors.push(`${method.toUpperCase()} ${path} does not document HTTP ${status}`);
        }
      }
    }
  }
}
if (missing.length || stale.length || contractErrors.length) {
  console.error(JSON.stringify({ missing_from_openapi: missing, missing_from_runtime: stale, contract_errors: contractErrors }, null, 2));
  process.exit(1);
}
console.log(`OpenAPI route contract matches ${expected.size} runtime operations; response and admin-auth requirements are documented.`);
