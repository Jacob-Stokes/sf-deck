package soqlauto

// Static literal data: SOQL keywords, aggregate/date functions,
// FIELDS() variants, and the full SOQL date-literal vocabulary.
//
// Matches Inspector Reloaded's hardcoded lists verbatim so the
// vocabulary stays current with what SF accepts.

// soqlKeywords are the bare clause keywords offered at top-level
// or as "what comes next?" prompts.
var soqlKeywords = []literal{
	{Value: "SELECT", Detail: "projection clause"},
	{Value: "FROM", Detail: "target sObject"},
	{Value: "WHERE", Detail: "row filter"},
	{Value: "AND", Detail: "filter conjunction"},
	{Value: "OR", Detail: "filter disjunction"},
	{Value: "NOT", Detail: "filter negation"},
	{Value: "LIKE", Detail: "wildcard match"},
	{Value: "IN", Detail: "value set match"},
	{Value: "NOT IN", Detail: "value set non-match"},
	{Value: "INCLUDES", Detail: "multipicklist contains any"},
	{Value: "EXCLUDES", Detail: "multipicklist contains none"},
	{Value: "ORDER BY", Detail: "sort clause"},
	{Value: "GROUP BY", Detail: "aggregation clause"},
	{Value: "HAVING", Detail: "post-aggregation filter"},
	{Value: "LIMIT", Detail: "row cap"},
	{Value: "OFFSET", Detail: "row skip"},
	{Value: "ASC", Detail: "ascending"},
	{Value: "DESC", Detail: "descending"},
	{Value: "NULLS FIRST", Detail: "null ordering"},
	{Value: "NULLS LAST", Detail: "null ordering"},
	{Value: "FOR VIEW", Detail: "mark records viewed"},
	{Value: "FOR REFERENCE", Detail: "mark records referenced"},
	{Value: "FOR UPDATE", Detail: "lock rows for update"},
	{Value: "WITH SECURITY_ENFORCED", Detail: "row + field security"},
	{Value: "WITH USER_MODE", Detail: "user permissions"},
	{Value: "WITH SYSTEM_MODE", Detail: "system permissions"},
	{Value: "USING SCOPE", Detail: "scope filter"},
}

// soqlFunctions are the aggregate, date, and formatter functions
// that can appear inside SELECT projections.
var soqlFunctions = []literal{
	// Aggregates
	{Value: "COUNT()", Detail: "row count"},
	{Value: "COUNT(Id)", Detail: "row count of Id"},
	{Value: "COUNT_DISTINCT()", Detail: "distinct value count"},
	{Value: "SUM()", Detail: "sum of numeric field"},
	{Value: "AVG()", Detail: "mean of numeric field"},
	{Value: "MIN()", Detail: "min value"},
	{Value: "MAX()", Detail: "max value"},
	// Date functions
	{Value: "CALENDAR_MONTH()", Detail: "month of date field"},
	{Value: "CALENDAR_QUARTER()", Detail: "quarter of date field"},
	{Value: "CALENDAR_YEAR()", Detail: "year of date field"},
	{Value: "DAY_IN_MONTH()", Detail: "day-of-month"},
	{Value: "DAY_IN_WEEK()", Detail: "day-of-week (1-7)"},
	{Value: "DAY_IN_YEAR()", Detail: "day-of-year"},
	{Value: "DAY_ONLY()", Detail: "strip time from datetime"},
	{Value: "FISCAL_MONTH()", Detail: "fiscal month"},
	{Value: "FISCAL_QUARTER()", Detail: "fiscal quarter"},
	{Value: "FISCAL_YEAR()", Detail: "fiscal year"},
	{Value: "HOUR_IN_DAY()", Detail: "hour of datetime"},
	{Value: "WEEK_IN_MONTH()", Detail: "week of month"},
	{Value: "WEEK_IN_YEAR()", Detail: "week of year"},
	// Formatters
	{Value: "FORMAT()", Detail: "localised format"},
	{Value: "convertCurrency()", Detail: "convert to corporate currency"},
	{Value: "toLabel()", Detail: "translated picklist label"},
	{Value: "convertTimezone()", Detail: "convert datetime tz"},
	// Field expressions
	{Value: "FIELDS(ALL)", Detail: "project every field (cap 200)"},
	{Value: "FIELDS(STANDARD)", Detail: "project standard fields"},
	{Value: "FIELDS(CUSTOM)", Detail: "project custom fields"},
}

// soqlDateLiterals are the dynamic-date constants accepted in
// WHERE/HAVING comparisons against date or datetime fields.
//
// The `:n` literals carry a placeholder `:1` so users can quickly
// edit the count.
var soqlDateLiterals = []literal{
	{Value: "TODAY"},
	{Value: "YESTERDAY"},
	{Value: "TOMORROW"},
	{Value: "LAST_WEEK"},
	{Value: "THIS_WEEK"},
	{Value: "NEXT_WEEK"},
	{Value: "LAST_MONTH"},
	{Value: "THIS_MONTH"},
	{Value: "NEXT_MONTH"},
	{Value: "LAST_90_DAYS"},
	{Value: "NEXT_90_DAYS"},
	{Value: "LAST_N_DAYS:1"},
	{Value: "NEXT_N_DAYS:1"},
	{Value: "N_DAYS_AGO:1"},
	{Value: "LAST_N_WEEKS:1"},
	{Value: "NEXT_N_WEEKS:1"},
	{Value: "LAST_N_MONTHS:1"},
	{Value: "NEXT_N_MONTHS:1"},
	{Value: "THIS_QUARTER"},
	{Value: "LAST_QUARTER"},
	{Value: "NEXT_QUARTER"},
	{Value: "LAST_N_QUARTERS:1"},
	{Value: "NEXT_N_QUARTERS:1"},
	{Value: "THIS_YEAR"},
	{Value: "LAST_YEAR"},
	{Value: "NEXT_YEAR"},
	{Value: "LAST_N_YEARS:1"},
	{Value: "NEXT_N_YEARS:1"},
	{Value: "THIS_FISCAL_QUARTER"},
	{Value: "LAST_FISCAL_QUARTER"},
	{Value: "NEXT_FISCAL_QUARTER"},
	{Value: "LAST_N_FISCAL_QUARTERS:1"},
	{Value: "NEXT_N_FISCAL_QUARTERS:1"},
	{Value: "THIS_FISCAL_YEAR"},
	{Value: "LAST_FISCAL_YEAR"},
	{Value: "NEXT_FISCAL_YEAR"},
	{Value: "LAST_N_FISCAL_YEARS:1"},
	{Value: "NEXT_N_FISCAL_YEARS:1"},
}

type literal struct {
	Value  string
	Detail string
}
