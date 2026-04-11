import type { JSONSchema, JSONSchemaProperty } from '../types/api';

interface SchemaFormProps {
  schema: JSONSchema;
  values: Record<string, any>;
  onChange: (values: Record<string, any>) => void;
  disabled?: boolean;
}

/**
 * SchemaForm dynamically renders a form from a JSON Schema.
 * Supports: string, integer, number, boolean, enum, object (key-value).
 * UI hints: x-ui-order, x-ui-placeholder, x-ui-widget, x-ui-hidden, x-ui-group.
 */
export default function SchemaForm({ schema, values, onChange, disabled }: SchemaFormProps) {
  const properties = schema.properties ?? {};
  const order: string[] = (schema['x-ui-order'] as string[]) ?? Object.keys(properties);
  const fields = order.filter(k => properties[k] && !(properties[k] as JSONSchemaProperty)['x-ui-hidden']);

  const setValue = (key: string, value: any) => {
    onChange({ ...values, [key]: value });
  };

  // Group fields
  const groups = new Map<string, string[]>();
  for (const key of fields) {
    const prop = properties[key] as JSONSchemaProperty;
    const group = (prop['x-ui-group'] as string) ?? '';
    if (!groups.has(group)) groups.set(group, []);
    groups.get(group)!.push(key);
  }

  return (
    <div>
      {[...groups.entries()].map(([group, keys]) => (
        <div key={group || '__default'}>
          {group && <h4 style={{ fontSize: '0.8rem', fontWeight: 600, color: '#374151', marginTop: '1rem', marginBottom: '0.5rem', borderBottom: '1px solid #e5e7eb', paddingBottom: 4 }}>{group}</h4>}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '0.75rem' }}>
            {keys.map(key => {
              const prop = properties[key] as JSONSchemaProperty;
              return <FieldRenderer key={key} name={key} prop={prop} value={values[key]} onChange={v => setValue(key, v)} disabled={disabled} />;
            })}
          </div>
        </div>
      ))}
    </div>
  );
}

function FieldRenderer({ name, prop, value, onChange, disabled }: {
  name: string;
  prop: JSONSchemaProperty;
  value: any;
  onChange: (v: any) => void;
  disabled?: boolean;
}) {
  const label = prop.title ?? name;
  const placeholder = (prop['x-ui-placeholder'] as string) ?? '';
  const widget = (prop['x-ui-widget'] as string) ?? '';
  const type = prop.type;

  // Boolean → checkbox (render inline, span full row for clarity)
  if (type === 'boolean') {
    return (
      <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: '0.8rem', cursor: disabled ? 'default' : 'pointer', color: '#374151' }}>
        <input
          type="checkbox"
          checked={value ?? prop.default ?? false}
          onChange={e => onChange(e.target.checked)}
          disabled={disabled}
        />
        {label}
        {prop.description && <span style={{ color: '#9ca3af', fontSize: '0.7rem' }} title={prop.description}>?</span>}
      </label>
    );
  }

  // Enum → select
  if (prop.enum && prop.enum.length > 0) {
    return (
      <div>
        <label style={lbl}>{label}</label>
        <select
          value={value ?? prop.default ?? ''}
          onChange={e => onChange(e.target.value)}
          disabled={disabled}
          style={inp}
        >
          {prop.enum.map(opt => (
            <option key={String(opt)} value={String(opt)}>{String(opt)}</option>
          ))}
        </select>
      </div>
    );
  }

  // Object with additionalProperties → key-value editor
  if (type === 'object' && prop.additionalProperties) {
    const entries: [string, string][] = Object.entries(value ?? {});
    const addRow = () => onChange({ ...(value ?? {}), '': '' });
    const removeRow = (k: string) => {
      const copy = { ...(value ?? {}) };
      delete copy[k];
      onChange(copy);
    };
    const updateRow = (oldKey: string, newKey: string, newVal: string) => {
      const copy: Record<string, string> = {};
      for (const [k, v] of Object.entries(value ?? {})) {
        copy[k === oldKey ? newKey : k] = k === oldKey ? newVal : (v as string);
      }
      onChange(copy);
    };

    return (
      <div style={{ gridColumn: '1 / -1' }}>
        <label style={lbl}>{label}</label>
        {entries.map(([k, v], i) => (
          <div key={i} style={{ display: 'flex', gap: 4, marginBottom: 4 }}>
            <input value={k} onChange={e => updateRow(k, e.target.value, v)} style={{ ...inp, flex: 1 }} placeholder="Key" disabled={disabled} />
            <input value={v} onChange={e => updateRow(k, k, e.target.value)} style={{ ...inp, flex: 2 }} placeholder="Value" disabled={disabled} />
            <button type="button" onClick={() => removeRow(k)} disabled={disabled}
              style={{ border: '1px solid #d1d5db', borderRadius: 4, background: '#fff', cursor: 'pointer', padding: '0 6px', color: '#ef4444', fontSize: '0.8rem' }}>x</button>
          </div>
        ))}
        <button type="button" onClick={addRow} disabled={disabled}
          style={{ border: '1px dashed #d1d5db', borderRadius: 4, background: '#f9fafb', cursor: 'pointer', padding: '2px 8px', fontSize: '0.75rem', color: '#6b7280' }}>+ Add</button>
      </div>
    );
  }

  // Textarea widget
  if (widget === 'textarea') {
    return (
      <div style={{ gridColumn: '1 / -1' }}>
        <label style={lbl}>{label}</label>
        <textarea
          value={value ?? prop.default ?? ''}
          onChange={e => onChange(e.target.value)}
          disabled={disabled}
          style={{ ...inp, minHeight: 60, resize: 'vertical' }}
          placeholder={placeholder}
        />
      </div>
    );
  }

  // Password widget
  if (widget === 'password') {
    return (
      <div>
        <label style={lbl}>{label}</label>
        <input
          type="password"
          value={value ?? ''}
          onChange={e => onChange(e.target.value)}
          disabled={disabled}
          style={inp}
          placeholder={placeholder}
        />
      </div>
    );
  }

  // Number / integer
  if (type === 'integer' || type === 'number') {
    return (
      <div>
        <label style={lbl}>{label}</label>
        <input
          type="number"
          value={value ?? prop.default ?? ''}
          onChange={e => {
            const v = e.target.value;
            onChange(v === '' ? undefined : (type === 'integer' ? parseInt(v) : parseFloat(v)));
          }}
          min={prop.minimum}
          max={prop.maximum}
          disabled={disabled}
          style={inp}
          placeholder={placeholder || (prop.default != null ? String(prop.default) : '')}
        />
      </div>
    );
  }

  // Default: string text input
  return (
    <div>
      <label style={lbl}>{label}</label>
      <input
        type="text"
        value={value ?? prop.default ?? ''}
        onChange={e => onChange(e.target.value)}
        disabled={disabled}
        style={inp}
        placeholder={placeholder || (prop.default != null ? String(prop.default) : '')}
      />
    </div>
  );
}

const lbl: React.CSSProperties = { display: 'block', fontSize: '0.75rem', fontWeight: 500, marginBottom: 4, color: '#374151' };
const inp: React.CSSProperties = { width: '100%', padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6, fontSize: '0.875rem', boxSizing: 'border-box' as const };
