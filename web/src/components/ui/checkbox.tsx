import * as React from "react";
import { Check } from "lucide-react";

import { cn } from "@/lib/utils";

export type CheckboxProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  "type"
>;

const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, disabled, ...props }, ref) => (
    <span className={cn("relative inline-flex h-4 w-4 shrink-0", className)}>
      <input
        ref={ref}
        type="checkbox"
        disabled={disabled}
        className="peer absolute inset-0 h-4 w-4 cursor-pointer opacity-0 disabled:cursor-not-allowed"
        {...props}
      />
      <span
        aria-hidden="true"
        className={cn(
          "pointer-events-none flex h-4 w-4 items-center justify-center rounded border border-border bg-background transition-colors",
          "peer-checked:border-neutral-500 peer-checked:bg-neutral-950 peer-checked:[&>svg]:opacity-100",
          "peer-focus-visible:outline-none peer-focus-visible:ring-1 peer-focus-visible:ring-ring",
          disabled && "opacity-50"
        )}
      >
        <Check className="h-3 w-3 text-white opacity-0 transition-opacity" />
      </span>
    </span>
  )
);
Checkbox.displayName = "Checkbox";

export { Checkbox };
