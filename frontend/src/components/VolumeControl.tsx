import { useId } from "react";
import "./volume-control.css";

/** Регулятор уровня с процентами, доступный с клавиатуры и для экранного диктора. */
export function VolumeControl({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
}) {
  const id = useId();
  const percent = Math.round(value * 100);
  return (
    <div className="volume-control">
      <div className="volume-control-heading">
        <label htmlFor={id}>{label}</label>
        <output htmlFor={id}>{percent}%</output>
      </div>
      <input
        id={id}
        className="volume-control-slider"
        type="range"
        min={0}
        max={100}
        step={1}
        value={percent}
        aria-valuetext={`${percent}%`}
        onChange={(event) => onChange(Number(event.target.value) / 100)}
      />
    </div>
  );
}
