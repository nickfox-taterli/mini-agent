#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""
xlsx_reader.py - Structure discovery and data analysis tool for Excel/CSV files.

Usage:
    python3 xlsx_reader.py <file1> [file2] [file3] ...    # analyze one or more files
    python3 xlsx_reader.py <file> --sheet Sales          # analyze one sheet
    python3 xlsx_reader.py <file> --json                  # machine-readable output
    python3 xlsx_reader.py <file> --quality               # data quality audit only

Supports: .xlsx, .xlsm, .xls, .csv, .tsv
Does NOT modify the source file in any way.

Exit codes:
    0 - success
    1 - file not found / unsupported format / encoding failure
"""

import sys
import json
import argparse
import warnings
from pathlib import Path

# Suppress warnings
warnings.filterwarnings('ignore', category=UserWarning)

import pandas as pd


# ---------------------------------------------------------------------------
# Format detection and loading
# ---------------------------------------------------------------------------

def detect_and_load(file_path: str, sheet_name_filter: str | None = None) -> dict:
    """
    Load file into {sheet_name: DataFrame} dict.
    CSV/TSV files are mapped to a single-key dict using the file stem as key.

    Raises ValueError for unsupported formats or encoding failures.
    """
    try:
        import pandas as pd
    except ImportError:
        raise RuntimeError(
            "pandas is not installed. Run: pip install pandas openpyxl"
        )

    path = Path(file_path)
    if not path.exists():
        raise FileNotFoundError(f"File not found: {file_path}")

    suffix = path.suffix.lower()

    if suffix in (".xlsx", ".xlsm"):
        target = sheet_name_filter if sheet_name_filter else None
        result = pd.read_excel(file_path, sheet_name=target)
        # pd.read_excel with sheet_name=None returns dict; with a name, returns DataFrame
        if isinstance(result, dict):
            return result
        else:
            return {sheet_name_filter: result}

    elif suffix == ".xls":
        # Support legacy .xls format using xlrd
        try:
            import xlrd
            import xlrd.xldate as xldate
        except ImportError:
            raise RuntimeError(
                ".xls format requires xlrd. Run: pip install xlrd==1.2.0"
            )
        
        workbook = xlrd.open_workbook(file_path)
        result = {}
        
        for sheet_index in range(workbook.nsheets):
            sheet = workbook.sheet_by_index(sheet_index)
            sheet_name = sheet.name
            
            # Filter by sheet name if specified
            if sheet_name_filter and sheet_name != sheet_name_filter:
                continue
            
            # Convert xlrd sheet to DataFrame
            data = []
            for row_index in range(sheet.nrows):
                row_data = []
                for col_index in range(sheet.ncols):
                    cell = sheet.cell(row_index, col_index)
                    cell_type = sheet.cell_type(row_index, col_index)
                    # xlrd cell types: 0=EMPTY, 1=TEXT, 2=NUMBER, 3=DATE, 4=BOOLEAN, 5=ERROR, 6=BLANK
                    if cell_type == 0:  # EMPTY
                        value = None
                    elif cell_type == 1:  # TEXT
                        value = str(cell.value)
                    elif cell_type == 2:  # NUMBER
                        value = cell.value
                    elif cell_type == 3:  # DATE
                        try:
                            value = xldate.xldate_as_datetime(cell.value, workbook.datemode)
                        except:
                            value = cell.value
                    elif cell_type == 4:  # BOOLEAN
                        value = bool(cell.value)
                    elif cell_type == 5:  # ERROR
                        value = f"#ERR{int(cell.value)}#"
                    elif cell_type == 6:  # BLANK
                        value = None
                    else:
                        value = cell.value
                    row_data.append(value)
                data.append(row_data)
            
            # Create DataFrame from data
            if data:
                # Use first row as header
                headers = data[0] if data else []
                if headers:
                    df = pd.DataFrame(data[1:], columns=headers)
                else:
                    df = pd.DataFrame(data)
                result[sheet_name] = df
            else:
                result[sheet_name] = pd.DataFrame()
        
        if not result and sheet_name_filter:
            raise ValueError(f"Sheet '{sheet_name_filter}' not found in {file_path}")
        return result

    elif suffix in (".csv", ".tsv"):
        sep = "\t" if suffix == ".tsv" else ","
        encodings = ["utf-8-sig", "gbk", "utf-8", "latin-1"]
        last_error = None
        for enc in encodings:
            try:
                df = pd.read_csv(file_path, sep=sep, encoding=enc)
                return {path.stem: df}
            except (UnicodeDecodeError, Exception) as e:
                last_error = e
                continue
        raise ValueError(
            f"Cannot decode {file_path}. Tried encodings: {encodings}. "
            f"Last error: {last_error}"
        )

    else:
        raise ValueError(
            f"Unsupported file format: {suffix}. "
            "Supported formats: .xlsx, .xlsm, .xls, .csv, .tsv"
        )


# ---------------------------------------------------------------------------
# Structure discovery
# ---------------------------------------------------------------------------

def explore_structure(sheets: dict) -> dict:
    """
    Return a structured dict describing each sheet.
    Keys: sheet_name -> {shape, columns, dtypes, null_counts, preview}
    """
    result = {}
    for sheet_name, df in sheets.items():
        if df.empty:
            result[sheet_name] = {
                "shape": {"rows": 0, "cols": 0},
                "columns": [],
                "dtypes": {},
                "null_columns": {},
                "preview": [],
            }
            continue
            
        null_counts = df.isnull().sum()
        null_info = {
            col: {"count": int(cnt), "pct": round(cnt / max(len(df), 1) * 100, 1)}
            for col, cnt in null_counts.items()
            if cnt > 0
        }
        result[sheet_name] = {
            "shape": {"rows": int(df.shape[0]), "cols": int(df.shape[1])},
            "columns": [str(c) for c in df.columns],
            "dtypes": {str(col): str(dtype) for col, dtype in df.dtypes.items()},
            "null_columns": null_info,
            "preview": df.head(5).to_dict(orient="records"),
        }
    return result


# ---------------------------------------------------------------------------
# Data quality audit
# ---------------------------------------------------------------------------

def audit_quality(sheets: dict) -> dict:
    """
    Return data quality findings per sheet.
    Checks: nulls, duplicates, mixed-type columns, potential year formatting issues.
    """
    import pandas as pd

    findings = {}
    for sheet_name, df in sheets.items():
        sheet_findings = []

        # Skip empty dataframes
        if df.empty:
            findings[sheet_name] = sheet_findings
            continue

        # Null values
        null_counts = df.isnull().sum()
        for col, cnt in null_counts.items():
            if cnt > 0:
                pct = round(cnt / max(len(df), 1) * 100, 1)
                sheet_findings.append({
                    "type": "null_values",
                    "column": str(col),
                    "count": int(cnt),
                    "pct": pct,
                    "note": f"Column '{col}' has {cnt} null values ({pct}%). "
                            "If this column contains Excel formulas, null values may "
                            "indicate that the formula cache has not been populated."
                })

        # Duplicate rows
        dup_count = int(df.duplicated().sum())
        if dup_count > 0:
            sheet_findings.append({
                "type": "duplicate_rows",
                "count": dup_count,
                "note": f"{dup_count} fully duplicate rows found."
            })

        # Mixed-type object columns (numeric data stored as text)
        try:
            obj_cols = df.select_dtypes(include=["object", "str"]).columns
            for col in obj_cols:
                col_data = df[col].dropna()
                if len(col_data) == 0:
                    continue
                try:
                    numeric_converted = pd.to_numeric(col_data, errors="coerce")
                    convertible = int(numeric_converted.notna().sum())
                    non_null_total = int(col_data.notna().sum())
                    if 0 < convertible < non_null_total:
                        sheet_findings.append({
                            "type": "mixed_type",
                            "column": str(col),
                            "convertible_to_numeric": convertible,
                            "non_convertible": non_null_total - convertible,
                            "note": f"Column '{col}' appears to contain mixed types: "
                                    f"{convertible} values can be parsed as numbers, "
                                    f"{non_null_total - convertible} cannot."
                        })
                except (TypeError, ValueError):
                    pass
        except Exception:
            pass

        # Year column formatting (e.g., 2024.0 stored as float)
        try:
            num_cols = df.select_dtypes(include="number").columns
            for col in num_cols:
                col_lower = str(col).lower()
                if "year" in col_lower or "yr" in col_lower or "年" in col_lower:
                    col_data = df[col].dropna()
                    if len(col_data) > 0 and col_data.between(1900, 2200).all():
                        if df[col].dtype == float:
                            sheet_findings.append({
                                "type": "year_as_float",
                                "column": str(col),
                                "note": f"Column '{col}' appears to be a year column stored as float."
                            })
        except Exception:
            pass

        # Outliers via IQR on numeric columns
        try:
            num_cols = df.select_dtypes(include="number").columns
            for col in num_cols:
                series = df[col].dropna()
                if len(series) < 4:
                    continue
                Q1, Q3 = series.quantile(0.25), series.quantile(0.75)
                IQR = Q3 - Q1
                if IQR == 0:
                    continue
                outlier_mask = (df[col] < Q1 - 1.5 * IQR) | (df[col] > Q3 + 1.5 * IQR)
                outlier_count = int(outlier_mask.sum())
                if outlier_count > 0:
                    sheet_findings.append({
                        "type": "outliers_iqr",
                        "column": str(col),
                        "count": outlier_count,
                        "note": f"Column '{col}' has {outlier_count} potential outlier(s)."
                    })
        except Exception:
            pass

        findings[sheet_name] = sheet_findings

    return findings


# ---------------------------------------------------------------------------
# Summary statistics
# ---------------------------------------------------------------------------

def compute_stats(sheets: dict) -> dict:
    """Compute descriptive statistics for numeric columns per sheet."""
    stats = {}
    for sheet_name, df in sheets.items():
        if df.empty:
            stats[sheet_name] = {}
            continue
        try:
            numeric_df = df.select_dtypes(include="number")
            if numeric_df.empty:
                stats[sheet_name] = {}
                continue
            desc = numeric_df.describe().round(4)
            stats[sheet_name] = desc.to_dict()
        except Exception:
            stats[sheet_name] = {}
    return stats


# ---------------------------------------------------------------------------
# Human-readable report rendering (single file)
# ---------------------------------------------------------------------------

def render_report(
    file_path: str,
    structure: dict,
    quality: dict,
    stats: dict,
) -> str:
    import pandas as pd
    
    lines = []
    p = lines.append

    p("=" * 60)
    p(f"ANALYSIS REPORT: {Path(file_path).name}")
    p("=" * 60)

    # File overview
    sheet_list = list(structure.keys())
    total_rows = sum(s["shape"]["rows"] for s in structure.values())
    p(f"\nSheets ({len(sheet_list)}): {', '.join(sheet_list)}")
    p(f"Total rows across all sheets: {total_rows:,}")

    for sheet_name, info in structure.items():
        p(f"\n{'─' * 50}")
        p(f"Sheet: {sheet_name}")
        p(f"{'─' * 50}")
        p(f"  Size: {info['shape']['rows']:,} rows × {info['shape']['cols']} cols")
        
        if info['columns']:
            p(f"  Columns: {info['columns'][:10]}{'...' if len(info['columns']) > 10 else ''}")

        # Data types
        if info["dtypes"]:
            p("\n  Column types:")
            for col, dtype in list(info["dtypes"].items())[:10]:
                p(f"    {col}: {dtype}")
            if len(info["dtypes"]) > 10:
                p(f"    ... and {len(info['dtypes']) - 10} more columns")

        # Nulls
        if info["null_columns"]:
            p("\n  Null values (columns with nulls only):")
            for col, null_info in info["null_columns"].items():
                p(f"    {col}: {null_info['count']} nulls ({null_info['pct']}%)")

        # Stats
        sheet_stats = stats.get(sheet_name, {})
        if sheet_stats:
            p("\n  Numeric column statistics:")
            numeric_cols = list(sheet_stats.keys())
            for col in numeric_cols[:6]:
                col_stats = sheet_stats[col]
                p(f"    {col}: count={col_stats.get('count', 'N/A')}, "
                  f"mean={col_stats.get('mean', 'N/A'):.2f}, "
                  f"min={col_stats.get('min', 'N/A'):.2f}, "
                  f"max={col_stats.get('max', 'N/A'):.2f}")
            if len(numeric_cols) > 6:
                p(f"    ... and {len(numeric_cols) - 6} more numeric columns")

        # Quality findings for this sheet
        sheet_quality = quality.get(sheet_name, [])
        if sheet_quality:
            p(f"\n  Data quality issues ({len(sheet_quality)} found):")
            for finding in sheet_quality:
                p(f"    [{finding['type'].upper()}] {finding['note']}")
        else:
            p("\n  Data quality: no issues found")

        # Preview
        if info["preview"]:
            p("\n  Preview (first 3 rows):")
            preview_df = pd.DataFrame(info["preview"][:3])
            for line in preview_df.to_string(index=False).splitlines():
                p(f"    {line}")

    p("\n" + "=" * 60)
    quality_issue_count = sum(len(v) for v in quality.values())
    if quality_issue_count == 0:
        p("RESULT: No data quality issues detected.")
    else:
        p(f"RESULT: {quality_issue_count} data quality issue(s) found.")
    p("=" * 60)

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Human-readable report rendering (multi-file)
# ---------------------------------------------------------------------------

def render_multi_report(results: list) -> str:
    """Render a combined report for multiple files."""
    import pandas as pd
    
    lines = []
    p = lines.append

    p("=" * 70)
    p("MULTI-FILE ANALYSIS REPORT")
    p("=" * 70)
    p(f"\nAnalyzed {len(results)} file(s):")
    for r in results:
        p(f"  - {r['file_name']}")
    
    # Summary table
    p("\n" + "─" * 70)
    p("SUMMARY TABLE")
    p("─" * 70)
    p(f"{'File':<30} {'Sheets':<10} {'Total Rows':<12} {'Issues':<8}")
    p("─" * 70)
    
    total_sheets = 0
    total_rows = 0
    total_issues = 0
    
    for r in results:
        file_name = r['file_name']
        sheets_count = len(r['structure'])
        rows_count = sum(s["shape"]["rows"] for s in r['structure'].values())
        issues_count = sum(len(v) for v in r['quality'].values())
        
        total_sheets += sheets_count
        total_rows += rows_count
        total_issues += issues_count
        
        p(f"{file_name:<30} {sheets_count:<10} {rows_count:<12,} {issues_count:<8}")
    
    p("─" * 70)
    p(f"{'TOTAL':<30} {total_sheets:<10} {total_rows:<12,} {total_issues:<8}")
    p("─" * 70)
    
    # Detailed reports for each file
    for r in results:
        p("\n\n")
        report = render_report(r['file_path'], r['structure'], r['quality'], r['stats'])
        p(report)
    
    p("\n" + "=" * 70)
    if total_issues == 0:
        p("RESULT: No data quality issues detected across all files.")
    else:
        p(f"RESULT: {total_issues} data quality issue(s) found across {len(results)} file(s).")
    p("=" * 70)

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------

def main() -> None:
    parser = argparse.ArgumentParser(
        description="Read and analyze Excel/CSV files without modifying them."
    )
    parser.add_argument("files", nargs="+", help="Path(s) to .xlsx, .xlsm, .xls, .csv, or .tsv file(s)")
    parser.add_argument("--sheet", help="Analyze a specific sheet only (applies to first file)", default=None)
    parser.add_argument(
        "--json", action="store_true", help="Output machine-readable JSON"
    )
    parser.add_argument(
        "--quality", action="store_true",
        help="Run data quality audit only (skip stats)"
    )
    parser.add_argument(
        "--summary", action="store_true",
        help="Show only summary table (for multi-file analysis)"
    )
    args = parser.parse_args()
    
    results = []
    
    for file_path in args.files:
        try:
            sheets = detect_and_load(file_path, sheet_name_filter=args.sheet)
        except (FileNotFoundError, ValueError, RuntimeError) as e:
            print(f"ERROR: {e}", file=sys.stderr)
            sys.exit(1)

        structure = explore_structure(sheets)
        quality = audit_quality(sheets)
        stats = {} if args.quality else compute_stats(sheets)
        
        results.append({
            'file_path': file_path,
            'file_name': Path(file_path).name,
            'structure': structure,
            'quality': quality,
            'stats': stats,
        })

    if args.json:
        output = {
            "files": [{
                "file": r['file_path'],
                "structure": r['structure'],
                "quality": r['quality'],
                "stats": r['stats'],
            } for r in results],
            "summary": {
                "total_files": len(results),
                "total_sheets": sum(len(r['structure']) for r in results),
                "total_rows": sum(sum(s["shape"]["rows"] for s in r['structure'].values()) for r in results),
                "total_quality_issues": sum(len(v) for r in results for v in r['quality'].values()),
            }
        }
        print(json.dumps(output, indent=2, ensure_ascii=False, default=str))
    elif len(results) == 1:
        # Single file - use detailed report
        report = render_report(args.files[0], results[0]['structure'], results[0]['quality'], results[0]['stats'])
        print(report)
    else:
        # Multiple files
        report = render_multi_report(results)
        print(report)


if __name__ == "__main__":
    main()
