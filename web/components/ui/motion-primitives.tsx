import * as React from 'react';
import {
  animate,
  motion,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
  type Transition,
} from 'motion/react';

import { cn } from '../../lib/utils';

/** Shared easing so everything in the console moves with one personality. */
export const EASE_OUT: Transition = { type: 'spring', stiffness: 260, damping: 30, mass: 0.7 };

/**
 * A number that springs to its new value instead of snapping.
 *
 * Metrics update every two seconds; snapping makes the whole dashboard flicker
 * and makes it hard to see which direction a value moved. Interpolating gives
 * the eye a direction to follow.
 */
export function AnimatedNumber({
  value,
  decimals = 0,
  suffix = '',
  className,
}: {
  value: number;
  decimals?: number;
  suffix?: string;
  className?: string;
}) {
  const reduced = useReducedMotion();
  const motionValue = useMotionValue(value);
  const spring = useSpring(motionValue, { stiffness: 120, damping: 22, mass: 0.6 });
  // The suffix is folded into the transform so the element has exactly one
  // text node. Rendering it as a sibling splits the text across two nodes,
  // which breaks any lookup that reads an element's own text -- Testing
  // Library's getByText among them.
  const display = useTransform(spring, (latest) => {
    const n = Number.isFinite(latest) ? latest : 0;
    return `${n.toFixed(decimals)}${suffix}`;
  });
  React.useEffect(() => {
    motionValue.set(value);
  }, [motionValue, value]);

  if (reduced) {
    return (
      <span className={cn('tabular', className)}>
        {value.toFixed(decimals)}
        {suffix}
      </span>
    );
  }

  // The MotionValue is rendered directly: motion subscribes and writes the
  // text node itself. This previously mirrored every spring frame into React
  // state, so each of the eight numbers on the Overview drove its own render
  // for roughly three quarters of every two-second cycle -- to change a string
  // that nothing else on the page depends on.
  // motion renders a MotionValue child by writing the DOM text node directly,
  // bypassing React entirely -- but only when it is the sole child.
  return <motion.span className={cn('tabular', className)}>{display}</motion.span>;
}

/** Children fade and rise in sequence rather than all at once. */
export function Stagger({
  className,
  children,
  delay = 0,
}: {
  className?: string;
  children: React.ReactNode;
  delay?: number;
}) {
  return (
    <motion.div
      className={className}
      initial="hidden"
      animate="show"
      variants={{
        hidden: {},
        show: { transition: { staggerChildren: 0.045, delayChildren: delay } },
      }}
    >
      {children}
    </motion.div>
  );
}

export function StaggerItem({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <motion.div
      className={className}
      variants={{
        hidden: { opacity: 0, y: 10 },
        show: { opacity: 1, y: 0, transition: EASE_OUT },
      }}
    >
      {children}
    </motion.div>
  );
}

/** Whole-page transition, used between routes. */
export function PageTransition({ children }: { children: React.ReactNode }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8 }}
      transition={{ duration: 0.22, ease: [0.22, 1, 0.36, 1] }}
    >
      {children}
    </motion.div>
  );
}

/**
 * Capacity bar whose fill animates and whose colour follows severity.
 *
 * Replaces a plain div whose width jumped between renders.
 */
export function Meter({
  value,
  tone = 'ok',
  className,
  label,
}: {
  value: number;
  tone?: 'ok' | 'warn' | 'crit' | 'primary';
  className?: string;
  label?: string;
}) {
  const clamped = Math.min(100, Math.max(0, Number.isFinite(value) ? value : 0));
  const fill =
    tone === 'crit'
      ? 'bg-crit'
      : tone === 'warn'
        ? 'bg-warn'
        : tone === 'primary'
          ? 'bg-primary'
          : 'bg-ok';

  return (
    <div
      className={cn('bg-muted/70 relative h-1.5 w-full overflow-hidden rounded-full', className)}
      role="meter"
      aria-valuenow={Math.round(clamped)}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={label}
    >
      <motion.div
        className={cn('h-full rounded-full', fill)}
        initial={false}
        animate={{ width: `${clamped}%` }}
        transition={EASE_OUT}
      />
    </div>
  );
}

/**
 * Compact sparkline. Gives every stat card a shape without the weight of a
 * full chart component.
 */
export function Sparkline({
  data,
  className,
  stroke = 'var(--primary)',
}: {
  data: number[];
  className?: string;
  stroke?: string;
}) {
  const gradientId = React.useId();

  const { points, area } = React.useMemo(() => {
    const values = data.filter((v) => Number.isFinite(v));
    if (values.length < 2) return { points: '', area: '' };

    const min = Math.min(...values);
    const max = Math.max(...values);
    const range = max - min;
    // Leave headroom so the line never touches the edges and gets clipped.
    const pad = 12;

    // Pure min/max normalisation turns a negligible change into a dramatic
    // mountain range: swap pegged at 99% moving by 4 MB looked like a cliff.
    // Only spread the series across the full height once the variation is
    // meaningful relative to the values themselves.
    const scale = Math.abs(max) > 0 ? range / Math.abs(max) : 0;
    const flat = range === 0 || scale < 0.02;

    const coords = values.map((v, i) => {
      const x = (i / (values.length - 1)) * 100;
      // Invert: SVG y grows downward.
      const y = flat ? 50 : pad + (1 - (v - min) / range) * (100 - pad * 2);
      return [x, y] as const;
    });

    const line = coords.map(([x, y]) => `${x.toFixed(2)},${y.toFixed(2)}`).join(' ');
    return { points: line, area: `0,100 ${line} 100,100` };
  }, [data]);

  if (!points) {
    return <div className={cn('h-8', className)} aria-hidden="true" />;
  }

  return (
    <svg
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      className={cn('h-10 w-full', className)}
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={stroke} stopOpacity={0.28} />
          <stop offset="100%" stopColor={stroke} stopOpacity={0} />
        </linearGradient>
      </defs>
      <motion.polygon
        points={area}
        fill={`url(#${gradientId})`}
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.5 }}
      />
      <motion.polyline
        points={points}
        fill="none"
        stroke={stroke}
        strokeWidth={2.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        initial={{ pathLength: 0, opacity: 0 }}
        animate={{ pathLength: 1, opacity: 1 }}
        transition={{ duration: 0.7, ease: 'easeOut' }}
      />
    </svg>
  );
}

/** A dot that pulses while the feed is live. */
export function LiveDot({ tone = 'ok' }: { tone?: 'ok' | 'warn' | 'crit' }) {
  const color = tone === 'crit' ? 'bg-crit' : tone === 'warn' ? 'bg-warn' : 'bg-ok';
  return (
    <span className="relative flex size-2">
      <motion.span
        className={cn('absolute inline-flex size-full rounded-full', color)}
        animate={{ opacity: [0.6, 0, 0.6], scale: [1, 2.2, 1] }}
        transition={{ duration: 2, repeat: Infinity, ease: 'easeOut' }}
      />
      <span className={cn('relative inline-flex size-2 rounded-full', color)} />
    </span>
  );
}

/** Imperatively animate a value — used for one-off emphasis. */
export function useCountUp(target: number, duration = 0.6) {
  const [value, setValue] = React.useState(0);
  React.useEffect(() => {
    const controls = animate(0, target, {
      duration,
      onUpdate: (v) => setValue(v),
    });
    return () => controls.stop();
  }, [target, duration]);
  return value;
}
