import type { ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import "./filter-select.css";

interface FilterSelectProps {
  label: string;
  value: string;
  options: readonly { value: string; label: string; disabled?: boolean }[];
  onChange: (value: string) => void;
  icon?: ReactNode;
  disabled?: boolean;
}

/** Размер фильтра следует за выбранной подписью, управление остаётся нативным. */
export function FilterSelect({
  label,
  value,
  options,
  onChange,
  icon,
  disabled = false,
}: FilterSelectProps) {
  const selected = options.find((option) => option.value === value);
  return (
    <span className="filter-select" data-disabled={disabled}>
      <span className="filter-select-content" aria-hidden="true">
        {icon}
        <span className="filter-select-text">{selected?.label}</span>
        <ChevronDown size={16} />
      </span>
      <select
        className="filter-select-input"
        aria-label={label}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option
            key={option.value}
            value={option.value}
            disabled={option.disabled}
          >
            {option.label}
          </option>
        ))}
      </select>
    </span>
  );
}
