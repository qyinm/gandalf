import React from 'react';
import { cn } from '../../lib/utils';

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'outline' | 'brand';
}

export function Badge({ className, variant = 'default', ...props }: BadgeProps) {
  return (
    <div
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
        variant === 'default' && 'bg-foreground text-background shadow hover:bg-foreground/80',
        variant === 'outline' && 'text-foreground border border-border bg-background hover:bg-surface-hover',
        variant === 'brand' && 'bg-brand-soft text-brand border border-brand/20 font-mono',
        className
      )}
      {...props}
    />
  );
}
