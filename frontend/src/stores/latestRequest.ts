interface Cancellable {
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
