# Chapter 4: Data — Tuples, Lists and Strings

Chapter 1 introduced tuples as the only compound type. Everything else Thunky
stores — lists, strings, optional values, trees — is a convention built on top
of them. This chapter covers those conventions, because every later chapter
depends on them.

---

## Lists are cons cells

A list is either:
- **The empty list**: the empty tuple `[]`.
- **A non-empty list**: a 2-tuple `[head, tail]`, called a **cons cell**, where `head` is the first element and `tail` is the rest of the list.

A three-element list `a, b, c` is therefore:

```thunky-static
[a, [b, [c, []]]]
```

This is the complete, desugared representation. Writing that by hand for every list would be painful, so Thunky provides syntactic sugar.

---

## List literals

The semicolon-separated form `[a; b; c]` is sugar for nested cons cells:

```thunky-static
[a; b; c]   ≡   [a, [b, [c, []]]]
[a; b]      ≡   [a, [b, []]]
[a;]        ≡   [a, []]          -- single-element list
[]          ≡   []               -- empty list (also empty tuple)
```

The separator — semicolon vs. comma — determines whether you get a list or a tuple:

| Expression  | What it is             |
|-------------|------------------------|
| `[a, b]`    | 2-element **tuple**    |
| `[a; b]`    | 2-element **list**     |
| `[a]`       | 1-element **tuple**    |
| `[a;]`      | 1-element **list**     |
| `[]`        | empty tuple = empty list |

**`[x;]` for a single-element list, always.** `[x]` is a tuple and will crash any function that tries to traverse it as a list.

---

## Pattern matching on lists

Since lists are 2-tuples, pattern matching uses tuple syntax:

```
let
  myHead = [h, t] -> h,
  myTail = [h, t] -> t
in
  show [myHead [1; 2; 3]; myTail [1; 2; 3]]    -- [1; [2; 3]]
```

The standard idiom: match `[]` for the empty case and `[h, t]` for the non-empty case.

```
let length = {
  []     -> 0,
  [h, t] -> length t > add 1
} in
  [1; 2; 3] > length > show    -- 3
```

---

## Strings are lists of code points

A string literal like `"hello"` is sugar for the list of its Unicode code points: `[104; 101; 108; 108; 111]`. Every list function works on strings.

To print text as characters (not as a list of numbers), use `write` instead of `show`:

```
write "hello"        -- prints: hello
```

```
show "hello"         -- prints: [104; 101; 108; 108; 111]
```

To obtain the code point of a single character, use `text.char` from the `text` module:

```
import text in
  text.char "A" > show    -- 65
```

---

## Comparing values: `eq` vs `equal`

`eq` is arithmetic. It compares two **numbers** and returns `1` or `0`; handing it anything else — a string, a tuple, a list — is a runtime error (`argument to eq is not a number`), so `eq "ab" "ab"` does not work. `equal` is the structural comparison: it walks both values and returns `1` only if they have the same shape with the same numbers at every leaf. Because a string is a list, `equal` is what you compare text with, and because list literals are only sugar, `equal` sees straight through them.

```
show [
  equal "cat" "cat",
  equal "cat" "cot",
  equal [1; 2; 3] [1, [2, [3, []]]],
  eq 3 3
]
```

Output: `[1, 0, 1, 1]`

Use `eq` on numbers — it is the cheaper primitive — and `equal` on everything else.

---

## Summary

- A list is `[]` (empty) or `[head, tail]` (cons cell).
- `[a; b; c]` is sugar for `[a, [b, [c, []]]]`.
- `[a;]` is a single-element list; `[a]` is a single-element tuple.
- Strings are lists of Unicode code points; use `write` to print them as text.
- `eq` compares numbers only; use `equal` for structural comparison, including strings.

---

## Exercises

### Exercise 4.1 — `[x]` and `[x;]`, diagnosed

Write one program that calls `list.length` on a one-element list, `list.length` on a two-element list, and `list.sum` on `[1; 2]`. Then change the one-element list from `[5;]` to `[5]` and run it again. Read the error message carefully: where does it point, and why is that misleading?

<details>
<summary>Solution</summary>

The working version — note the trailing semicolon in `[5;]`:

```
import list in
  show [list.length [5;], list.length [5; 6], list.sum [1; 2]]
```

Output: `[1, 2, 3]`

Written as `[5]`, that argument is a **1-tuple**, not a list, and the program fails at runtime:

```thunky-static
import list in
  show (list.length [5])
```

```text
core/list.þ:23:2: no pattern matched value [5]
	[] -> 0,
	^^^^^^^^
```

The lesson is in the location: `core/list.þ`. The blame lands on the standard library, inside `length`'s own `[] -> 0` case, because that is where the mismatch is actually detected — `[5]` is neither `[]` nor a cons cell `[h, t]`, so no case matches. Your code, which is where the mistake is, is not mentioned at all. Whenever an error points into `core/`, suspect the shape of the value you passed in, and check your one-element lists first.

</details>

---

### Exercise 4.2 — Palindrome check

Write `isPalindrome` that returns `1` if a list reads the same forwards and backwards.

<details>
<summary>Solution</summary>

```
import list in
let isPalindrome = xs -> equal xs (xs > list.reverse) in
  show [
    isPalindrome [1; 2; 3; 2; 1],
    isPalindrome [1; 2; 3],
    isPalindrome []
  ]
```

Output: `[1, 0, 1]`

</details>

---
