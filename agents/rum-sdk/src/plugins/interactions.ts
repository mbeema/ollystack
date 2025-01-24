/**
 * User Interactions Plugin
 *
 * Tracks user interactions like clicks, inputs, and form submissions.
 * Provides insight into user behavior and engagement.
 */

import { RUMPlugin, RUMSDKInterface, InteractionData } from '../types';

export class InteractionsPlugin implements RUMPlugin {
  name = 'interactions';

  private sdk: RUMSDKInterface | null = null;
  private boundHandlers: Map<string, EventListener> = new Map();
  private lastInteractionTime = 0;
  private interactionQueue: InteractionData[] = [];
  private flushTimeout: ReturnType<typeof setTimeout> | null = null;

  // Debounce/throttle settings
  private minInteractionInterval = 100; // ms between interactions
  private flushInterval = 1000; // Batch flush interval

  init(sdk: RUMSDKInterface): void {
    this.sdk = sdk;
    this.sdk.log('debug', 'Interactions plugin initializing');

    if (typeof window === 'undefined') return;

    this.setupClickTracking();
    this.setupInputTracking();
    this.setupFormTracking();
    this.setupScrollTracking();
  }

  destroy(): void {
    if (typeof window === 'undefined') return;

    // Remove all event listeners
    for (const [event, handler] of this.boundHandlers) {
      document.removeEventListener(event, handler, true);
    }
    this.boundHandlers.clear();

    if (this.flushTimeout) {
      clearTimeout(this.flushTimeout);
    }

    // Flush remaining interactions
    this.flushInteractions();
  }

  private setupClickTracking(): void {
    const handler = (event: Event) => this.handleClick(event as MouseEvent);
    document.addEventListener('click', handler, true);
    this.boundHandlers.set('click', handler);
  }

  private setupInputTracking(): void {
    // Track blur on inputs to capture final values
    const blurHandler = (event: Event) => this.handleInputBlur(event as FocusEvent);
    document.addEventListener('blur', blurHandler, true);
    this.boundHandlers.set('blur', blurHandler);

    // Track change events for selects and checkboxes
    const changeHandler = (event: Event) => this.handleChange(event);
    document.addEventListener('change', changeHandler, true);
    this.boundHandlers.set('change', changeHandler);
  }

  private setupFormTracking(): void {
    const handler = (event: Event) => this.handleSubmit(event as SubmitEvent);
    document.addEventListener('submit', handler, true);
    this.boundHandlers.set('submit', handler);
  }

  private setupScrollTracking(): void {
    let scrollDepth = 0;
    let lastScrollReport = 0;

    const handler = this.throttle(() => {
      const scrollTop = window.scrollY || document.documentElement.scrollTop;
      const docHeight = document.documentElement.scrollHeight - window.innerHeight;
      const currentDepth = docHeight > 0 ? Math.round((scrollTop / docHeight) * 100) : 0;

      // Report at 25%, 50%, 75%, 90%, 100% milestones
      const milestones = [25, 50, 75, 90, 100];
      for (const milestone of milestones) {
        if (currentDepth >= milestone && scrollDepth < milestone && lastScrollReport !== milestone) {
          lastScrollReport = milestone;
          this.queueInteraction({
            type: 'scroll',
            target: 'document',
            targetTag: 'document',
            value: `${milestone}%`,
            timestamp: Date.now(),
          });
          break;
        }
      }

      scrollDepth = Math.max(scrollDepth, currentDepth);
    }, 100);

    window.addEventListener('scroll', handler, { passive: true });
    this.boundHandlers.set('scroll', handler as EventListener);
  }

  private handleClick(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    if (!target) return;

    // Skip if too soon after last interaction
    const now = Date.now();
    if (now - this.lastInteractionTime < this.minInteractionInterval) return;
    this.lastInteractionTime = now;

    // Get meaningful click target (traverse up to find interactive element)
    const interactiveTarget = this.findInteractiveParent(target);
    const element = interactiveTarget || target;

    const interaction: InteractionData = {
      type: 'click',
      target: this.getTargetSelector(element),
      targetId: element.id || undefined,
      targetClass: element.className ? String(element.className).split(' ')[0] : undefined,
      targetTag: element.tagName.toLowerCase(),
      targetText: this.getTargetText(element),
      x: event.clientX,
      y: event.clientY,
      timestamp: now,
    };

    this.queueInteraction(interaction);
  }

  private handleInputBlur(event: FocusEvent): void {
    const target = event.target as HTMLInputElement | HTMLTextAreaElement;
    if (!target || !['INPUT', 'TEXTAREA'].includes(target.tagName)) return;

    // Don't track password fields
    if (target.type === 'password') return;

    // Check if value actually changed
    const currentValue = target.value;
    const initialValue = target.getAttribute('data-ollystack-initial');

    if (initialValue === null) {
      // First focus, store initial value
      target.setAttribute('data-ollystack-initial', currentValue);
      return;
    }

    if (currentValue === initialValue) return;

    // Value changed, record interaction
    target.setAttribute('data-ollystack-initial', currentValue);

    const interaction: InteractionData = {
      type: 'input',
      target: this.getTargetSelector(target),
      targetId: target.id || undefined,
      targetClass: target.className ? String(target.className).split(' ')[0] : undefined,
      targetTag: target.tagName.toLowerCase(),
      value: this.sanitizeInputValue(target),
      timestamp: Date.now(),
    };

    this.queueInteraction(interaction);
  }

  private handleChange(event: Event): void {
    const target = event.target as HTMLInputElement | HTMLSelectElement;
    if (!target) return;

    // Handle select, radio, checkbox
    if (!['SELECT', 'INPUT'].includes(target.tagName)) return;

    const inputTarget = target as HTMLInputElement;
    if (target.tagName === 'INPUT' && !['radio', 'checkbox'].includes(inputTarget.type)) return;

    let value: string | undefined;
    if (target.tagName === 'SELECT') {
      value = (target as HTMLSelectElement).options[(target as HTMLSelectElement).selectedIndex]?.text;
    } else if (inputTarget.type === 'checkbox') {
      value = inputTarget.checked ? 'checked' : 'unchecked';
    } else if (inputTarget.type === 'radio') {
      value = inputTarget.value;
    }

    const interaction: InteractionData = {
      type: 'change',
      target: this.getTargetSelector(target),
      targetId: target.id || undefined,
      targetTag: target.tagName.toLowerCase(),
      value,
      timestamp: Date.now(),
    };

    this.queueInteraction(interaction);
  }

  private handleSubmit(event: SubmitEvent): void {
    const form = event.target as HTMLFormElement;
    if (!form) return;

    const interaction: InteractionData = {
      type: 'submit',
      target: this.getTargetSelector(form),
      targetId: form.id || undefined,
      targetTag: 'form',
      value: form.action ? new URL(form.action, window.location.href).pathname : undefined,
      timestamp: Date.now(),
    };

    this.queueInteraction(interaction);
  }

  private queueInteraction(interaction: InteractionData): void {
    this.interactionQueue.push(interaction);

    // Schedule flush
    if (!this.flushTimeout) {
      this.flushTimeout = setTimeout(() => {
        this.flushInteractions();
      }, this.flushInterval);
    }
  }

  private flushInteractions(): void {
    if (this.flushTimeout) {
      clearTimeout(this.flushTimeout);
      this.flushTimeout = null;
    }

    if (this.interactionQueue.length === 0) return;

    const interactions = this.interactionQueue.splice(0);

    for (const interaction of interactions) {
      this.sdk?.sendEvent({
        type: 'interaction',
        data: interaction,
      });
    }

    this.sdk?.log('debug', `Flushed ${interactions.length} interactions`);
  }

  private findInteractiveParent(element: HTMLElement): HTMLElement | null {
    let current: HTMLElement | null = element;
    const interactiveTags = ['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA', 'LABEL'];

    while (current && current !== document.body) {
      if (interactiveTags.includes(current.tagName)) {
        return current;
      }
      if (current.getAttribute('role') === 'button') {
        return current;
      }
      if (current.onclick || current.getAttribute('data-action')) {
        return current;
      }
      current = current.parentElement;
    }

    return null;
  }

  private getTargetSelector(element: HTMLElement): string {
    // Build a simple selector
    const parts: string[] = [];

    // Tag name
    parts.push(element.tagName.toLowerCase());

    // ID if present
    if (element.id) {
      parts.push(`#${element.id}`);
      return parts.join('');
    }

    // First class if present
    if (element.className && typeof element.className === 'string') {
      const firstClass = element.className.split(' ')[0];
      if (firstClass && !firstClass.startsWith('ng-') && !firstClass.startsWith('_')) {
        parts.push(`.${firstClass}`);
      }
    }

    // Data attributes
    const dataTestId = element.getAttribute('data-testid') || element.getAttribute('data-test-id');
    if (dataTestId) {
      parts.push(`[data-testid="${dataTestId}"]`);
    }

    return parts.join('');
  }

  private getTargetText(element: HTMLElement): string | undefined {
    let text: string | undefined;

    // For buttons and links, get inner text
    if (['A', 'BUTTON'].includes(element.tagName)) {
      text = element.innerText?.trim();
    }

    // For inputs, get placeholder or label
    if (element.tagName === 'INPUT') {
      const input = element as HTMLInputElement;
      text = input.placeholder || this.findLabelText(input);
    }

    // Truncate long text
    if (text && text.length > 50) {
      text = text.substring(0, 50) + '...';
    }

    return text || undefined;
  }

  private findLabelText(input: HTMLInputElement): string | undefined {
    // Try to find associated label
    if (input.id) {
      const label = document.querySelector(`label[for="${input.id}"]`);
      if (label) {
        return label.textContent?.trim();
      }
    }

    // Check parent label
    const parentLabel = input.closest('label');
    if (parentLabel) {
      return parentLabel.textContent?.trim();
    }

    return undefined;
  }

  private sanitizeInputValue(input: HTMLInputElement | HTMLTextAreaElement): string | undefined {
    // Don't capture actual values for sensitive fields
    const sensitiveTypes = ['password', 'email', 'tel', 'ssn', 'credit-card'];
    const sensitiveNames = ['password', 'email', 'phone', 'ssn', 'credit', 'card', 'cvv', 'secret', 'token'];

    if (sensitiveTypes.includes(input.type)) {
      return `[${input.type}]`;
    }

    const name = (input.name || input.id || '').toLowerCase();
    for (const sensitive of sensitiveNames) {
      if (name.includes(sensitive)) {
        return `[${input.type || 'text'}]`;
      }
    }

    // Return length indicator instead of actual value for longer inputs
    if (input.value.length > 20) {
      return `[${input.value.length} chars]`;
    }

    return input.value;
  }

  private throttle<T extends (...args: unknown[]) => void>(
    fn: T,
    limit: number
  ): (...args: Parameters<T>) => void {
    let inThrottle = false;
    return (...args: Parameters<T>) => {
      if (!inThrottle) {
        fn(...args);
        inThrottle = true;
        setTimeout(() => (inThrottle = false), limit);
      }
    };
  }
}

export function createInteractionsPlugin(): RUMPlugin {
  return new InteractionsPlugin();
}
