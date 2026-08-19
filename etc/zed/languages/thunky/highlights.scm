; ----------------------------------------------------------------- keywords
(import_clause "import" @keyword)
(import_clause "in"     @keyword)
(let_expr      "let"    @keyword)
(let_expr      "in"     @keyword)
(module_declaration "module" @keyword)

; ----------------------------------------------------------------- builtins
((identifier) @function.builtin
 (#match? @function.builtin
  "^(add|mul|sub|div|fdiv|mod|fmod|pow|sqrt|eq|lt|lte|gte|gt|neq|equal|eval|peek|show|write|bwrite|stdin|bstdin)$"))

; ----------------------------------------------------------------- operators
(operation operator: _ @operator)
(lambda_arrow "->" @operator)
(lambda_case  "->" @operator)

; ----------------------------------------------------------------- literals
(number)  @number
(string)  @string
(comment) @comment

; ----------------------------------------------------------------- names
(binding    name:   (identifier) @variable)
(lambda_arrow pattern: (identifier) @variable.parameter)
(lambda_case  pattern: (identifier) @variable.parameter)
(qualified_name module: (identifier) @namespace
                name:   (identifier) @property)
(identifier) @variable
