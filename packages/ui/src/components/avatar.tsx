import * as React from 'react'
import { cn } from '../lib/utils'

interface AvatarContextValue {
  imageLoaded: boolean
  setImageLoaded: (loaded: boolean) => void
}

const AvatarContext = React.createContext<AvatarContextValue | undefined>(undefined)

const Avatar = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => {
  const [imageLoaded, setImageLoaded] = React.useState(false)

  return (
    <AvatarContext.Provider value={{ imageLoaded, setImageLoaded }}>
      <div
        ref={ref}
        className={cn(
          'relative flex h-10 w-10 shrink-0 overflow-hidden rounded-full bg-zinc-800',
          className
        )}
        {...props}
      />
    </AvatarContext.Provider>
  )
})
Avatar.displayName = 'Avatar'

const AvatarImage = React.forwardRef<
  HTMLImageElement,
  React.ImgHTMLAttributes<HTMLImageElement>
>(({ className, src, alt = '', onLoad, onError, ...props }, ref) => {
  const context = React.useContext(AvatarContext)

  if (!context) {
    return null
  }

  const { imageLoaded, setImageLoaded } = context

  return (
    <img
      ref={ref}
      src={src}
      alt={alt}
      className={cn(
        'aspect-square h-full w-full object-cover transition-opacity',
        imageLoaded ? 'opacity-100' : 'opacity-0',
        className
      )}
      onLoad={(e) => {
        setImageLoaded(true)
        onLoad?.(e)
      }}
      onError={(e) => {
        setImageLoaded(false)
        onError?.(e)
      }}
      {...props}
    />
  )
})
AvatarImage.displayName = 'AvatarImage'

const AvatarFallback = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, children, ...props }, ref) => {
  const context = React.useContext(AvatarContext)

  if (!context || context.imageLoaded) {
    return null
  }

  return (
    <div
      ref={ref}
      className={cn(
        'flex h-full w-full items-center justify-center rounded-full bg-zinc-700 text-sm font-medium text-zinc-200',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
})
AvatarFallback.displayName = 'AvatarFallback'

export { Avatar, AvatarImage, AvatarFallback }
