// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The reference, read from the contract.
 *
 * `api/openapi.yaml` is the source and this is one rendering of it (ADR-0004). The document
 * arrives through `@hubtask/api-client` as JSON with its author's key order kept - so the order
 * of operations, parameters and responses on a page is the order somebody wrote them in, which is
 * the one that reads.
 *
 * What this module decides, so the pages do not: which operations belong to a tag (its first tag;
 * every operation in this contract has exactly one), how a schema becomes rows a `ParameterTable`
 * can draw, how deep an object is unfolded before its name stands for it, and what a `curl` line
 * for an operation looks like. Everything here is pure and tested in plain Node.
 */

import { firstSentence } from './markdown.ts';

/* ── The document, as much of it as this reads ──────────────────────────────────────────── */

export interface Schema {
  readonly $ref?: string;
  readonly type?: string | readonly string[];
  readonly format?: string;
  readonly description?: string;
  readonly properties?: Readonly<Record<string, Schema>>;
  readonly required?: readonly string[];
  readonly items?: Schema;
  readonly enum?: readonly unknown[];
  readonly default?: unknown;
  readonly example?: unknown;
  readonly allOf?: readonly Schema[];
  readonly oneOf?: readonly Schema[];
  readonly anyOf?: readonly Schema[];
  readonly deprecated?: boolean;
  readonly minimum?: number;
  readonly maximum?: number;
  readonly maxLength?: number;
  readonly minLength?: number;
  readonly maxItems?: number;
  readonly pattern?: string;
  readonly additionalProperties?: boolean | Schema;
  readonly nullable?: boolean;
  readonly const?: unknown;
}

export interface ParameterObject {
  readonly $ref?: string;
  readonly name?: string;
  readonly in?: 'path' | 'query' | 'header' | 'cookie';
  readonly required?: boolean;
  readonly description?: string;
  readonly deprecated?: boolean;
  readonly schema?: Schema;
}

export interface MediaType {
  readonly schema?: Schema;
  readonly example?: unknown;
}

export interface OperationObject {
  readonly operationId?: string;
  readonly tags?: readonly string[];
  readonly summary?: string;
  readonly description?: string;
  readonly deprecated?: boolean;
  readonly parameters?: readonly ParameterObject[];
  readonly requestBody?: { readonly $ref?: string; readonly description?: string; readonly required?: boolean; readonly content?: Readonly<Record<string, MediaType>> };
  readonly responses?: Readonly<Record<string, { readonly $ref?: string; readonly description?: string; readonly content?: Readonly<Record<string, MediaType>> }>>;
  readonly security?: readonly Readonly<Record<string, readonly string[]>>[];
}

export interface Document {
  readonly info: { readonly title: string; readonly version: string; readonly description?: string };
  readonly servers?: readonly { readonly url: string }[];
  readonly tags?: readonly { readonly name: string; readonly description?: string }[];
  readonly paths: Readonly<Record<string, Readonly<Record<string, OperationObject | readonly ParameterObject[] | undefined>>>>;
  readonly components: {
    readonly schemas?: Readonly<Record<string, Schema>>;
    readonly parameters?: Readonly<Record<string, ParameterObject>>;
    readonly responses?: Readonly<Record<string, { readonly description?: string; readonly content?: Readonly<Record<string, MediaType>> }>>;
    readonly securitySchemes?: Readonly<Record<string, { readonly type: string; readonly scheme?: string; readonly description?: string }>>;
  };
}

/* ── What a page renders ──────────────────────────────────────────────────────────────── */

export type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS';
const METHODS: readonly string[] = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options'];

export interface Row {
  readonly name: string;
  readonly type: string;
  readonly isRequired: boolean;
  readonly description: string;
  readonly facts?: readonly string[];
  readonly isDeprecated?: boolean;
  readonly children?: readonly Row[];
}

export interface Body {
  readonly contentType: string;
  /** The schema's name, where it has one - a link into the schemas page. */
  readonly schemaName?: string;
  readonly rows: readonly Row[];
  readonly example?: string;
}

export interface Response {
  readonly status: string;
  readonly description: string;
  readonly body?: Body;
}

export interface Operation {
  readonly id: string;
  readonly method: Method;
  readonly path: string;
  readonly summary: string;
  readonly description: string;
  readonly isDeprecated: boolean;
  readonly pathParameters: readonly Row[];
  readonly queryParameters: readonly Row[];
  readonly headerParameters: readonly Row[];
  readonly requestBody?: Body;
  readonly responses: readonly Response[];
  /** `false` when the operation declares `security: []` - the public routes. */
  readonly needsBearer: boolean;
  /** The message codes the description names, in backticks: `sync.cursor_too_old`. */
  readonly codes: readonly string[];
  readonly curl: string;
}

export interface TagPage {
  readonly name: string;
  readonly slug: string;
  readonly description: string;
  readonly lede: string;
  readonly operations: readonly Operation[];
}

export interface NamedSchema {
  readonly name: string;
  readonly description: string;
  readonly type: string;
  readonly rows: readonly Row[];
  readonly facts: readonly string[];
}

export interface Reference {
  readonly title: string;
  readonly version: string;
  readonly description: string;
  readonly baseUrl: string;
  readonly tags: readonly TagPage[];
  readonly schemas: readonly NamedSchema[];
  readonly operationCount: number;
}

/* ── Reading ───────────────────────────────────────────────────────────────────────────── */

/** How deep an object is unfolded into rows before its name stands for it. */
const UNFOLD_DEPTH = 2;

function refName(ref: string | undefined): string | undefined {
  if (!ref) return undefined;
  const last = ref.lastIndexOf('/');
  return last === -1 ? ref : ref.slice(last + 1);
}

function isOperation(value: unknown): value is OperationObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export class Reader {
  readonly #document: Document;

  constructor(document: Document) {
    this.#document = document;
  }

  /** A `$ref` to a component schema, or the schema itself. Cycles are the caller's to bound. */
  resolveSchema(schema: Schema | undefined): Schema {
    if (!schema) return {};
    if (schema.$ref) {
      const name = refName(schema.$ref) ?? '';
      const found = this.#document.components.schemas?.[name];
      if (!found) throw new Error(`reference: unknown schema ${schema.$ref}`);
      return found;
    }
    return schema;
  }

  resolveParameter(parameter: ParameterObject): ParameterObject {
    if (!parameter.$ref) return parameter;
    const name = refName(parameter.$ref) ?? '';
    const found = this.#document.components.parameters?.[name];
    if (!found) throw new Error(`reference: unknown parameter ${parameter.$ref}`);
    return found;
  }

  /** The type as a reader spells it: `string (uuid)`, `array of Label`, `one of A, B`, `Thing`. */
  typeOf(schema: Schema): string {
    if (schema.$ref) return refName(schema.$ref) ?? 'object';
    if (schema.allOf) {
      const named = schema.allOf.map((part) => refName(part.$ref)).filter((name): name is string => !!name);
      return named.length > 0 ? named.join(' + ') : 'object';
    }
    const union = schema.oneOf ?? schema.anyOf;
    if (union) return `one of ${union.map((part) => this.typeOf(part)).join(', ')}`;
    if (schema.type === 'array' || (Array.isArray(schema.type) && schema.type.includes('array'))) {
      return `array of ${schema.items ? this.typeOf(schema.items) : 'anything'}`;
    }
    const types = Array.isArray(schema.type) ? schema.type : schema.type ? [schema.type] : [];
    const own = types.filter((type) => type !== 'null');
    const base = own.length > 0 ? own.join(' or ') : schema.properties ? 'object' : 'anything';
    const nullable = types.includes('null') || schema.nullable ? ', or null' : '';
    return schema.format ? `${base} (${schema.format})${nullable}` : `${base}${nullable}`;
  }

  /** The small facts beneath a type: the values of an enum, a default, a bound, a pattern. */
  factsOf(schema: Schema): string[] {
    const facts: string[] = [];
    if (schema.enum) facts.push(...schema.enum.map((value) => String(value)));
    if (schema.const !== undefined) facts.push(`const: ${String(schema.const)}`);
    if (schema.default !== undefined) facts.push(`default: ${JSON.stringify(schema.default)}`);
    if (schema.minimum !== undefined) facts.push(`minimum: ${schema.minimum}`);
    if (schema.maximum !== undefined) facts.push(`maximum: ${schema.maximum}`);
    if (schema.minLength !== undefined) facts.push(`minLength: ${schema.minLength}`);
    if (schema.maxLength !== undefined) facts.push(`maxLength: ${schema.maxLength}`);
    if (schema.maxItems !== undefined) facts.push(`maxItems: ${schema.maxItems}`);
    if (schema.pattern !== undefined) facts.push(`pattern: ${schema.pattern}`);
    return facts;
  }

  /** The properties of an object schema, `allOf` merged, as rows; unfolded to a bounded depth. */
  rowsOf(schema: Schema, depth = 0): Row[] {
    const resolved = this.resolveSchema(schema);
    if (resolved.allOf) {
      return resolved.allOf.flatMap((part) => this.rowsOf(part, depth));
    }
    const properties = resolved.properties ?? {};
    const required = new Set(resolved.required ?? []);
    return Object.entries(properties).map(([name, property]) => {
      const target = this.resolveSchema(property);
      const facts = this.factsOf(target);
      const description = firstSentence(property.description ?? target.description ?? '');
      const row: Row = {
        name,
        type: this.typeOf(property),
        isRequired: required.has(name),
        description,
        ...(facts.length > 0 ? { facts } : {}),
        ...(property.deprecated || target.deprecated ? { isDeprecated: true } : {}),
      };
      // An object with its own fields is unfolded one level; past the bound its name stands for
      // it, and the schemas page carries the rest. A `$ref` at the bound stays a name for the
      // same reason, which is also what keeps a recursive schema finite.
      const inner = this.#objectWithin(property);
      if (inner && depth < UNFOLD_DEPTH) {
        const children = this.rowsOf(inner, depth + 1);
        if (children.length > 0) return { ...row, children };
      }
      return row;
    });
  }

  /** The object a property is or holds: itself, or an array's items, resolved. */
  #objectWithin(schema: Schema): Schema | undefined {
    const resolved = this.resolveSchema(schema);
    if (resolved.properties || resolved.allOf) return resolved;
    if (resolved.items) {
      const item = this.resolveSchema(resolved.items);
      if (item.properties || item.allOf) return item;
    }
    return undefined;
  }

  /** An example value built from what the schema declares, for the `curl` line. */
  exampleOf(schema: Schema, depth = 0): unknown {
    const resolved = this.resolveSchema(schema);
    if (resolved.example !== undefined) return resolved.example;
    if (schema.example !== undefined) return schema.example;
    if (resolved.default !== undefined) return resolved.default;
    if (resolved.const !== undefined) return resolved.const;
    if (resolved.enum && resolved.enum.length > 0) return resolved.enum[0];
    if (resolved.allOf) {
      return Object.assign({}, ...resolved.allOf.map((part) => this.exampleOf(part, depth)));
    }
    const union = resolved.oneOf ?? resolved.anyOf;
    if (union && union.length > 0) return this.exampleOf(union[0] ?? {}, depth);
    const types = Array.isArray(resolved.type) ? resolved.type : resolved.type ? [resolved.type] : [];
    const type = types.find((candidate) => candidate !== 'null') ?? (resolved.properties ? 'object' : 'string');
    switch (type) {
      case 'object': {
        if (depth >= 3) return {};
        const out: Record<string, unknown> = {};
        const required = new Set(resolved.required ?? []);
        for (const [name, property] of Object.entries(resolved.properties ?? {})) {
          // Required fields always; optional ones only at the top, so an example stays a
          // request somebody would send rather than every field the schema knows.
          if (required.has(name) || depth === 0) out[name] = this.exampleOf(property, depth + 1);
        }
        return out;
      }
      case 'array':
        return depth >= 3 ? [] : [this.exampleOf(resolved.items ?? {}, depth + 1)];
      case 'integer':
        return resolved.minimum ?? 1;
      case 'number':
        return resolved.minimum ?? 1.5;
      case 'boolean':
        return true;
      default:
        switch (resolved.format) {
          case 'uuid':
            return '018f6e2a-2b7c-7c1e-9b1a-3d5e6f7a8b9c';
          case 'date-time':
            return '2026-09-16T10:00:00Z';
          case 'date':
            return '2026-09-16';
          case 'email':
            return 'anna@example.org';
          case 'uri':
            return 'https://example.org';
          default:
            return 'string';
        }
    }
  }

  #parameters(operation: OperationObject, pathItem: Readonly<Record<string, unknown>>): ParameterObject[] {
    const shared = Array.isArray(pathItem['parameters']) ? (pathItem['parameters'] as ParameterObject[]) : [];
    return [...shared, ...(operation.parameters ?? [])].map((parameter) => this.resolveParameter(parameter));
  }

  #parameterRows(parameters: readonly ParameterObject[], where: ParameterObject['in']): Row[] {
    return parameters
      .filter((parameter) => parameter.in === where)
      .map((parameter) => {
        const schema = this.resolveSchema(parameter.schema);
        const facts = this.factsOf(schema);
        return {
          name: parameter.name ?? '',
          type: this.typeOf(parameter.schema ?? {}),
          isRequired: parameter.required === true || where === 'path',
          description: firstSentence(parameter.description ?? ''),
          ...(facts.length > 0 ? { facts } : {}),
          ...(parameter.deprecated ? { isDeprecated: true } : {}),
        };
      });
  }

  #body(content: Readonly<Record<string, MediaType>> | undefined, withExample: boolean): Body | undefined {
    if (!content) return undefined;
    const [contentType, media] = Object.entries(content)[0] ?? [];
    if (!contentType || !media) return undefined;
    const schema = media.schema ?? {};
    const rows = schema.$ref || schema.properties || schema.allOf ? this.rowsOf(schema) : [];
    const schemaName = refName(schema.$ref);
    let example: string | undefined;
    if (withExample && contentType.endsWith('json')) {
      const value = media.example ?? this.exampleOf(schema);
      example = JSON.stringify(value, null, 2);
    }
    return { contentType, ...(schemaName ? { schemaName } : {}), rows, ...(example ? { example } : {}) };
  }

  #responses(operation: OperationObject): Response[] {
    return Object.entries(operation.responses ?? {}).map(([status, response]) => {
      let description = response.description ?? '';
      let content = response.content;
      if (response.$ref) {
        const found = this.#document.components.responses?.[refName(response.$ref) ?? ''];
        description = found?.description ?? description;
        content = found?.content ?? content;
      }
      const body = this.#body(content, false);
      return { status, description: firstSentence(description), ...(body ? { body } : {}) };
    });
  }

  curlOf(method: Method, path: string, parameters: readonly ParameterObject[], body: Body | undefined, needsBearer: boolean): string {
    const lines = [`curl -X ${method} "$HUBTASK_URL${path}"`];
    if (needsBearer) lines.push('-H "Authorization: Bearer $HUBTASK_TOKEN"');
    for (const parameter of parameters) {
      if (parameter.in !== 'header') continue;
      if (parameter.name === 'Idempotency-Key') lines.push('-H "Idempotency-Key: $(uuidgen)"');
      else if (parameter.name === 'If-Match') lines.push('-H \'If-Match: "<the ETag you read>"\'');
    }
    if (body?.example) {
      lines.push(`-H "Content-Type: ${body.contentType}"`);
      lines.push(`-d '${body.example.replace(/'/g, "'\\''")}'`);
    }
    return lines.join(' \\\n  ');
  }

  operation(path: string, method: string, raw: OperationObject, pathItem: Readonly<Record<string, unknown>>): Operation {
    const parameters = this.#parameters(raw, pathItem);
    const needsBearer = !(Array.isArray(raw.security) && raw.security.length === 0);
    const requestBody = this.#body(raw.requestBody?.content, true);
    const upper = method.toUpperCase() as Method;
    const description = raw.description ?? '';
    const codes = [...new Set([...description.matchAll(/`([a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+)`/g)].map((match) => match[1] ?? ''))];
    return {
      id: raw.operationId ?? `${method}-${path}`,
      method: upper,
      path,
      // Ninety operations of the contract carry no summary; the identifier, written out, is a
      // truer line than an empty one, and it is marked as derived nowhere because it is the
      // operation's own name.
      summary: raw.summary ?? humanize(raw.operationId ?? `${method} ${path}`),
      description,
      isDeprecated: raw.deprecated === true,
      pathParameters: this.#parameterRows(parameters, 'path'),
      queryParameters: this.#parameterRows(parameters, 'query'),
      headerParameters: this.#parameterRows(parameters, 'header'),
      ...(requestBody ? { requestBody } : {}),
      responses: this.#responses(raw),
      needsBearer,
      codes,
      curl: this.curlOf(upper, path, parameters, requestBody, needsBearer),
    };
  }

  /** Every operation, in the document's order, each under its first tag. */
  read(): Reference {
    const byTag = new Map<string, Operation[]>();
    let operationCount = 0;
    for (const [path, item] of Object.entries(this.#document.paths)) {
      for (const [method, raw] of Object.entries(item)) {
        if (!METHODS.includes(method) || !isOperation(raw)) continue;
        const operation = this.operation(path, method, raw, item as Readonly<Record<string, unknown>>);
        const tag = raw.tags?.[0] ?? 'other';
        byTag.set(tag, [...(byTag.get(tag) ?? []), operation]);
        operationCount++;
      }
    }
    // Declared tags first, in their declared order; a tag an operation uses and nobody declared
    // follows, because an undeclared tag is still where the operation lives.
    const declared = this.#document.tags ?? [];
    const names = [...declared.map((tag) => tag.name), ...[...byTag.keys()].filter((name) => !declared.some((tag) => tag.name === name))];
    const tags: TagPage[] = names
      .filter((name) => (byTag.get(name)?.length ?? 0) > 0)
      .map((name) => {
        const description = declared.find((tag) => tag.name === name)?.description ?? '';
        return { name, slug: slugOf(name), description, lede: firstSentence(description), operations: byTag.get(name) ?? [] };
      });
    const schemas: NamedSchema[] = Object.entries(this.#document.components.schemas ?? {}).map(([name, schema]) => ({
      name,
      description: schema.description ?? '',
      type: this.typeOf(schema),
      // Flat: on the schemas page every named schema has its own table, and a field that is one
      // is a name the reader scrolls to rather than a second copy of that table.
      rows: this.rowsOf(schema, UNFOLD_DEPTH),
      facts: this.factsOf(schema),
    }));
    const server = this.#document.servers?.[0]?.url ?? '';
    return {
      title: this.#document.info.title,
      version: this.#document.info.version,
      description: this.#document.info.description ?? '',
      baseUrl: server,
      tags,
      schemas,
      operationCount,
    };
  }
}

/** `createWorkItem` → `Create work item`. */
export function humanize(identifier: string): string {
  const words = identifier.replace(/([a-z0-9])([A-Z])/g, '$1 $2').toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function slugOf(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
}

export function readReference(document: Document): Reference {
  return new Reader(document).read();
}
