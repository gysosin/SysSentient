import * as React from 'react';

import { cn } from '../../lib/utils';

function Card({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card"
      className={cn(
        // A flat panel on the ambient grid, not a floating glass card. The
        // backdrop-blur and translucency the old theme used cost a compositing
        // layer per card on a page that repaints every two seconds, and they
        // muddied the hairline dividers this design is built on.
        'bg-panel text-card-foreground border-line flex flex-col rounded-xl border',
        className,
      )}
      {...props}
    />
  );
}

function CardHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-header"
      className={cn('flex items-center gap-3 px-5 pt-4 pb-3', className)}
      {...props}
    />
  );
}

function CardTitle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-title"
      // The display face, uppercase and tracked out. Every panel title in the
      // console speaks with this voice; it is most of what separates an
      // instrument from an admin template.
      className={cn(
        'font-display text-base font-semibold uppercase leading-none tracking-[0.08em]',
        className,
      )}
      {...props}
    />
  );
}

function CardDescription({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-description"
      className={cn('text-muted-foreground text-xs', className)}
      {...props}
    />
  );
}

function CardAction({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="card-action" className={cn('ml-auto', className)} {...props} />;
}

function CardContent({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="card-content" className={cn('px-5 pb-5', className)} {...props} />;
}

export { Card, CardHeader, CardTitle, CardDescription, CardAction, CardContent };
