export interface Cancellable {
  cancel(): void | PromiseLike<void>
}

export class LatestRequest {
  private active?: Cancellable

  replace(request: Cancellable) {
    this.clear()
    this.active = request
    return () => this.active === request
  }

  finish(request: Cancellable) {
    if (this.active === request) this.active = undefined
  }

  clear() {
    const previous = this.active
    this.active = undefined
    if (previous) void Promise.resolve(previous.cancel()).catch(() => undefined)
  }
}

/** Runs several requests as one cancellable unit for LatestRequest; cancelling
 *  it cancels every member that supports cancellation. */
export function cancellableAll<T extends readonly unknown[]>(
  ...calls: { [K in keyof T]: PromiseLike<T[K]> & Partial<Cancellable> }
): Promise<T> & Cancellable {
  const members = calls as readonly (PromiseLike<unknown> & Partial<Cancellable>)[]
  const combined = Promise.all(members) as unknown as Promise<T> & Cancellable
  combined.cancel = () => {
    for (const call of members) void Promise.resolve(call.cancel?.()).catch(() => undefined)
  }
  return combined
}
