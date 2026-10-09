import type { ReactNode } from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { cn } from '@/lib/utils'

// A centered modal on the same primitive as Sheet.
export function Dialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  children?: ReactNode
  className?: string
}) {
  return (
    <DialogPrimitive.Root open={props.open} onOpenChange={(open) => props.onOpenChange(open)}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className='fixed inset-0 z-50 bg-black/30 transition-opacity duration-150 data-ending-style:opacity-0 data-starting-style:opacity-0' />
        <DialogPrimitive.Popup
          className={cn(
            'bg-popover text-popover-foreground fixed top-1/2 left-1/2 z-50 flex w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 flex-col gap-4 rounded-xl border p-6 text-sm shadow-lg transition duration-150 data-ending-style:opacity-0 data-starting-style:opacity-0',
            props.className
          )}
        >
          <DialogPrimitive.Title className='text-base font-semibold'>{props.title}</DialogPrimitive.Title>
          {props.description && (
            <DialogPrimitive.Description className='text-muted-foreground text-sm leading-6'>
              {props.description}
            </DialogPrimitive.Description>
          )}
          {props.children}
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
